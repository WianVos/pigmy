package migrate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"

	"github.com/schollz/progressbar"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	jira "github.com/wianvos/go-jira"
	utils "github.com/wianvos/pigmy/cmd/utils"
	gitlab "github.com/xanzy/go-gitlab"
)

var projectName string

var (
	gitlabUsers = make(map[string]*gitlab.User)
)

var chunksize int

const tmpPassword = "dummy12345"
const retry = 3
const retryTimeSeconds = 5
const limit = 0

//create the command and add it to the migrateCMD objects
func addProject() {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "migrate an entire project from jira to gitlab",
		Run:   runProject,
	}

	migrateCMD.AddCommand(cmd)

}

func runProject(cmd *cobra.Command, args []string) {

	contextLogger = contextLogger.WithFields(log.Fields{"subcommand": "Project"})
	//check if we received an argument
	if len(args) != 1 {
		contextLogger.Fatal("need a project name to actually migrate stuff")
		os.Exit(2)
	}

	projectName = args[0]

	contextLogger = contextLogger.WithFields(log.Fields{"Project": projectName})

	// to fully migrate a project we will need to migrate the following parts
	// users
	// issues
	// notes
	// attachements

	//The way we are going about this is simpel.
	//Collect all the messages in full (a little memory consuming .. but hye .. mem is cheap)
	// filter out all the users
	// create them (so much easier then doing that on a per message basis)
	// migrate all the issues ( as the correct user using sudo)
	// migrate the notes ( as the correct user using sudo )
	// migrate attachements ( you guessed it )

	// first lets fetch the issues belonging to the project
	p := fetchProject()

	p.MigrateProject()

}

//Project holds all the project goodies
type Project struct {
	Pid    int
	Name   string
	Issues Issues
	Users  Users
}

// Issues holds everything we need to recreate the exact issue in gitlab
type Issue struct {
	CreatorID    string
	JiraID       string
	Title        string
	Description  string
	Status       string
	Assignee     string
	AssigneeIDs  []int
	Labels       []string
	CreatedAt    time.Time
	Comments     Comments
	Attachements Attachements
}

type Comment struct {
	Body      string
	CreatorID string
}

type Attachement struct {
	FileName  string
	CreatorID string
}

type User struct {
	Email    string
	Username string
	Name     string
	Password string
	UID      int
}

type Users []User
type Comments []Comment
type Attachements []Attachement
type Issues []Issue

func fetchProject() Project {

	// procure a jira client object
	jlc := utils.GetJiraClient()

	// compose search query
	// qc := jira.GetQueryOptions{Fields: "comment"}
	// qa := jira.GetQueryOptions{Fields: "attachment"}

	// get all issues related to the project

	jql := fmt.Sprintf("project = %s", projectName)

	fmt.Println("starting collection of Jira Issues")

	var index int
	index = 0
	var issues jira.Issues

	for {
		if limit != 0 {
			chunksize = limit
		} else {
			chunksize = 1000
		}
		contextLogger.Infof("retrieving jira issues %d - %d", index, index+chunksize)
		//Compose the jira query
		so := jira.SearchOptions{MaxResults: chunksize, StartAt: index}

		//Retrieve the issues from the project
		is, resp, err := jlc.Issue.Search(jql, &so)
		if err != nil {
			contextLogger.Errorln(err)
			contextLogger.Errorln(resp.StatusCode)
			fmt.Println("unable to retrieve issues")
			os.Exit(2)
		}

		contextLogger.Infof("retrieved: %d issues", len(is))

		issues = append(issues, is...)
		fmt.Printf("retrieved %d issues\n", len(issues))
		//TODO: take this out before production

		if len(is) < 1000 {
			break
		} else {
			index = index + 1000
		}

		if len(issues) > limit && limit != 0 {
			break
		}

	}

	contextLogger.Infof("found: %d issues", len(issues))
	fmt.Printf("found %d issues associated to project: %s \n ", len(issues), projectName)

	//initialize project

	p := Project{Name: projectName}

	//Feedback is everything .. let's start a progressbar
	bar := progressbar.New(len(issues))

	// and let's keep track of errors encountered
	var ec int
	ec = 0

	// initialize the issue struct
	var gIssues Issues
	contextLogger.Infoln("starting the retrieval of the issues from JIRA")

	// loop over issues and propegate them into the project object
	for _, i := range issues {
		//init logger
		contextLogger := contextLogger.WithFields(log.Fields{"Jira Issue": i.ID})
		//add one to the progressbar
		bar.Add(1)

		// get entire issue
		// first get the right query options

		// ji, _, err := jlc.Issue.Get(i.ID, &jira.GetQueryOptions{Fields: "created"})
		ji, _, err := jlc.Issue.Get(i.ID, nil)
		if err != nil {
			contextLogger.Errorln("unable to retrieve issue")
			ec = ec + 1
		}

		contextLogger.Debug(ji)

		// log the found issue
		contextLogger.Infoln("found issue")
		gi := Issue{
			CreatedAt:    time.Time(ji.Fields.Created),
			CreatorID:    i.Fields.Creator.Name,
			Title:        fmt.Sprintf("%s:%s", i.Key, i.Fields.Summary),
			Description:  i.Fields.Description,
			Labels:       []string{"To Do"},
			Comments:     getComments(ji),
			Attachements: getAttachements(ji),
			JiraID:       ji.ID,
			Status:       ji.Fields.Status.Name,
			Assignee:     ji.Fields.Assignee.Name,
		}
		if gi.CreatorID == "admin" {
			gi.CreatorID = "root"
		}

		gIssues = append(gIssues, gi)
	}

	p.Issues = gIssues

	p.PopulateUsers()

	if ec != 0 {
		contextLogger.Errorf("encountered %d errors while retrieving project from jira", ec)
	}
	return p
}

func getComments(ji *jira.Issue) Comments {
	c := Comments{}
	if ji.Fields.Comments != nil {
		contextLogger.Infof("found %d comments", len(ji.Fields.Comments.Comments))

		for _, co := range ji.Fields.Comments.Comments {
			contextLogger.WithFields(log.Fields{"JiraComment": co.ID}).Info("attempting to create")

			jn := Comment{
				Body:      co.Body,
				CreatorID: co.Author.Name,
			}

			c = append(c, jn)

			contextLogger.WithFields(log.Fields{"JiraComment": co.ID}).Info("comment read")
		}

		return c
	}

	return c
}

func getAttachements(ji *jira.Issue) Attachements {

	a := Attachements{}
	jlc := utils.GetJiraClient()

	if ji.Fields.Attachments != nil {
		contextLogger.Infof("found %d attachements", len(ji.Fields.Attachments))

		for _, ao := range ji.Fields.Attachments {
			contextLogger.WithFields(log.Fields{"JiraAttachement": ao.ID}).Info("attempting to retrieve")

			// download attachement file
			resp, err := jlc.Issue.DownloadAttachment(ao.ID)

			if err != nil {
				contextLogger.WithFields(log.Fields{"attachementID": ao.ID}).Error(err)
				break
			}

			contextLogger.WithFields(log.Fields{"JiraAttachement": ao.ID}).Info("file downloaded")

			tf := utils.GetTmpDirFileName(ao.Filename)
			outFile, err := os.Create(tf)
			if err != nil {
				contextLogger.Error(err)
				break
			}

			defer outFile.Close()

			_, err = io.Copy(outFile, resp.Body)
			if err != nil {
				contextLogger.WithFields(log.Fields{"JiraAttachement": ao.ID}).Error(err)
				break

			}
			contextLogger.WithFields(log.Fields{"JiraAttachement": ao.ID}).Infof("file saved to disk: %s", tf)

			ja := Attachement{
				CreatorID: ao.Author.Name,
				FileName:  tf,
			}

			a = append(a, ja)

		}
	}

	return a
}

//PopulateUsers loops over all the project assets and get the userid's responsible for creating them, rendering a struct with all users involved in the project
func (p *Project) PopulateUsers() {

	var users Users

	fmt.Println("\ngetting all user id's associated with this project")
	b := progressbar.New(len(p.Issues))

	for _, i := range p.Issues {
		if i.CreatorID == "root" {
			contextLogger.Debugln("found root while searching for user")
		}
		if !users.containsUser(i.CreatorID) {
			u, err := jiraGetUser(i.CreatorID)
			if err == nil {
				users = append(users, u)
			}
		}

		for _, c := range i.Comments {
			if !users.containsUser(c.CreatorID) {
				u, err := jiraGetUser(c.CreatorID)
				if err == nil {
					users = append(users, u)
				}
			}
		}
		for _, a := range i.Attachements {
			if !users.containsUser(a.CreatorID) {
				u, err := jiraGetUser(a.CreatorID)
				if err == nil {
					users = append(users, u)
				}
			}
		}

		b.Add(1)

	}
	fmt.Println("\n")

	contextLogger.Info("found the following users")
	for _, u := range users {
		contextLogger.WithFields(log.Fields{"name": u.Name, "username": u.Username}).Info("found and added to the project")
	}
	p.Users = users
}

// check if a users already exists
func (u *Users) containsUser(n string) bool {
	for _, l := range *u {

		if l.Username == n {
			return true
		}
	}
	return false
}

// retrieve a user from jira
func jiraGetUser(n string) (User, error) {

	contextLogger.Infof("searching for user %s", n)

	jlc := utils.GetJiraClient()
	ju, _, err := jlc.User.Find(n)

	if err != nil {
		contextLogger.Errorf("unable to retrieve data for user %s", n)

		contextLogger.Error(err)

		return User{}, err

	}

	if len(ju) != 1 {

		contextLogger.Errorf("Found to many users when searching for user %s", n)
		return User{}, err
	}

	u := User{
		Email:    ju[0].EmailAddress,
		Username: ju[0].Name,
		Name:     ju[0].DisplayName,
	}

	return u, nil
}

func (p *Project) MigrateProject() {

	glc := utils.GetGitlabClient()

	contextLogger = contextLogger.WithField("project", p.Name)
	fmt.Println("starting project migration")
	// does the project exist in gitlab ??
	lpo := gitlab.ListProjectsOptions{Search: &p.Name}
	pl, _, err := glc.Projects.ListProjects(&lpo, nil)
	if err != nil {
		contextLogger.Errorf("unable to query gitlab for projects")
	}

	vv := gitlab.PublicVisibility
	if len(pl) == 0 {
		contextLogger.Debugf("project needs to be created")
		_, _, err := glc.Projects.CreateProject(&gitlab.CreateProjectOptions{Name: &p.Name, Visibility: &vv})
		if err != nil {
			contextLogger.Errorf("unable to create project in Gitlab")
			contextLogger.Error(err)
			fmt.Println("unable to create project ... this is not good... duh .. exiting")
			os.Exit(99)
		}
	}
	if len(pl) > 1 {
		contextLogger.Debugf("multiple projects found .. it is not wise to continue")
		fmt.Println("multiple projects found .. exiting.. we don't do rando stuff")
		os.Exit(1)
	}

	if len(pl) == 1 {
		contextLogger.Infof("project found")
	}

	// now lets retrieve the project id .. we need it further down the line
	p.getPID()

	// if so create the users first

	err = p.Users.Create(p.Pid)
	if err != nil {
		contextLogger.Error("unable to migrate users.. ")
		fmt.Println("unable to migrate users .. exiting")
		os.Exit(2)
	}

	// now migrate the issues with notes and attachements

	p.MigrateIssues()
}

//Create creates the collection of users in gitlab
func (u *Users) Create(p int) error {
	contextLogger.Infoln("creating users")

	fmt.Println("migrating users")
	bar := progressbar.New(len(*u))

	for _, user := range *u {
		if user.Username != "admin" && user.Username != "root" {
			err := user.Create(p)
			if err != nil {
				return err
			}
		}
		bar.Add(1)
	}
	fmt.Printf("\n")
	return nil
}

//Create creates a user in gitlab if it does nog exist
func (u *User) Create(p int) error {
	// setup the context logger
	contextLogger = contextLogger.WithField("user", u.Username)

	// retrieve gilab client
	glc := utils.GetGitlabClient()

	//setup variables
	var cu *gitlab.User
	var err error

	// if the username is empty ... where dealing with root .. and root don't need no attention so skip
	if u.Username != "" {

		// try to get the user from gitlab
		cu = gitlabUserGet(u.Username)

		// if the returned user object is nil .. it means the user does not exist in gitlab so where creating it ..
		if cu == nil {
			// set a tmp password .. (constant)
			tp := tmpPassword
			a := true
			// set the useroptions
			gcuo := gitlab.CreateUserOptions{
				Email:    &u.Email,
				Username: &u.Username,
				Name:     &u.Name,
				Password: &tp,
				Admin:    &a,
			}

			contextLogger.Debug("starting creation")
			// create the user
			cu, _, err = glc.Users.CreateUser(&gcuo, nil)
			//handle error
			if err != nil {
				contextLogger.Error(err)
				contextLogger.Errorln("unable to create")
				return err
			}
		} else {
			contextLogger.Infof("user already exists")
		}

		//add the user to our project
		err = gitlabAddUserToProjectAsMaster(cu.ID, p)

		if err != nil {
			contextLogger.Error(err)
			contextLogger.Errorf("unable to add user to project")
			return err
		}

		contextLogger.Infof("user added to project")
	}

	return nil
}

func gitlabUserGet(us string) *gitlab.User {

	// lets see if we searched for this user before
	// to do this we store every user we find in gitlab in this map and search that before we go to the actual system ..
	for n, u := range gitlabUsers {
		if n == us {
			return u
		}
	}

	// we didn't find the user yet so let's proceed and look for it
	glc := utils.GetGitlabClient()
	so := gitlab.ListUsersOptions{
		Search: &us,
	}

	contextLogger = contextLogger.WithField("username", us)

	ul, _, err := glc.Users.ListUsers(&so, nil)
	if err != nil {
		log.Errorf("unable to search for user %s", us)
		return nil
	}

	if len(ul) > 1 {
		log.Errorf("multiple users found for %s. no feasable way to determine outcome returning false", us)
		return nil
	}

	if len(ul) < 1 {
		log.Debugf("no user found for %s. returning false", us)
		return nil
	}

	log.Debugf("user: %s exists", us)

	// the user exists so let's put it in the map mentioned above first
	gitlabUsers[ul[0].Username] = ul[0]

	// now return the damm thing

	return ul[0]

}

func gitlabAddUserToProjectAsMaster(u, p int) error {

	glc := utils.GetGitlabClient()

	perm := gitlab.MasterPermissions

	_, resp, err := glc.ProjectMembers.AddProjectMember(p, &gitlab.AddProjectMemberOptions{
		UserID:      &u,
		AccessLevel: &perm,
	})

	if resp.StatusCode == 409 {
		contextLogger.Debug(err)
		contextLogger.Debugf("unable to add user to project")
		return nil

	}

	if err != nil {
		contextLogger.Errorln(err)
		contextLogger.Errorf("unable to add user to project as Master")
		return errors.New("unable to add user to project")
	}

	return nil

}

func (p *Project) getPID() {
	glc := utils.GetGitlabClient()

	pl, _, err := glc.Projects.ListProjects(nil, nil)
	if err != nil {
		contextLogger.Error(err)
		contextLogger.Errorf("unable to retrieve project ID")
		os.Exit(1)
	}

	for _, pj := range pl {
		if pj.Name == p.Name {
			p.Pid = pj.ID
		}
	}
	contextLogger.Infof("retrieved PID %d for project", p.Pid)

}

//Issue stuff below

//MigrateIssues actually migrates the issue to gitlab.
func (p *Project) MigrateIssues() {

	contextLogger.Info("starting issue creation")
	fmt.Println("Migrating Issues")
	// x := 1
	bar := progressbar.New(len(p.Issues))
	s := 0
	e := 0

	for _, i := range p.Issues {

		bar.Add(1)
		err := i.Create(p)
		if err != nil {
			e = e + 1
			f := utils.GetTmpDirFileName(i.JiraID)
			contextLogger.Errorf("dumping jira record %s to file %s for further investigation", i.JiraID, f)
			utils.WriteToFile(utils.RenderJSON(i), f)
		} else {
			s = s + 1
		}

	}
	fmt.Printf("project migrated. %d issues migrated succesfully. %d errors encountered", s, e)

}

//Create creates a single issue .. what did u expect from a function called create ??
func (i *Issue) Create(p *Project) error {
	contextLogger := log.WithFields(log.Fields{"JiraIssueID": i.JiraID})

	contextLogger.Infof("start migration")

	glc := utils.GetGitlabClient()

	var assigneeIDs []int
	var ai []int
	var o *gitlab.Issue
	var err error
	var resp *gitlab.Response

	// see if the issue already exists in gitlab
	si, _, err := glc.Issues.ListProjectIssues(p.Pid, &gitlab.ListProjectIssuesOptions{Search: &i.Title})
	if err != nil {
		fmt.Println("lp: err not nil")
	}
	if len(si) != 0 {
		contextLogger.WithField("issue title", si[0].Title).Infoln("issue found skipping migration")
		return nil
	}

	// compose the list of assignee's
	if au := gitlabUserGet(i.Assignee); au != nil {
		ai = append(ai, au.ID)
		assigneeIDs = ai
	} else {
		ai = append(ai, 1)
		assigneeIDs = ai
	}

	if au := gitlabUserGet(i.CreatorID); au == nil {
		i.CreatorID = "1"
	}

	rc := 0
	// dropping the note into gitlab .. like it's hot
	for {
		d := translateText(i.Description)
		o, resp, err = glc.Issues.CreateIssue(
			p.Pid,
			&gitlab.CreateIssueOptions{
				Title:       &i.Title,
				Description: &d,
				AssigneeIDs: assigneeIDs,
				Labels:      []string{"To Do"},
				CreatedAt:   &i.CreatedAt,
			}, gitlab.WithSudo(i.CreatorID))

		if err != nil {
			contextLogger.WithError(err).Errorf("unable to create issue in gitlab, will retry in %d seconds", retryTimeSeconds)
			contextLogger.Error(spew.Sdump(resp))
			contextLogger.Error(spew.Sdump(o))
			rc = rc + 1
			if rc == retry {
				contextLogger.Error("unable to complete request using retry window .. moving on to the next issue")
				return err
				break
			}
		} else {
			contextLogger.Debug(o)
			contextLogger.Infof("issues created")
			break
		}

		contextLogger.Debugf("retry count: %d", rc)
		rc = rc + 1
		time.Sleep(retryTimeSeconds * time.Second)
	}

	// handeling the comments
	for x, c := range i.Comments {
		b := translateText(c.Body)
		contextLogger := contextLogger.WithField("comment", x)
		in, _, err := glc.Notes.CreateIssueNote(
			p.Pid,
			o.IID,
			&gitlab.CreateIssueNoteOptions{Body: &b},
			gitlab.WithSudo(i.CreatorID))

		if err != nil {
			contextLogger.Error(err)
			contextLogger.Errorln("unable to create Comment")
			return err
		}
		contextLogger.Debug(in)
		contextLogger.Infoln("comment created")

	}

	//attachements... don't get too attached .. that's what my momma used to say :-)
	for _, a := range i.Attachements {
		contextLogger := contextLogger.WithField("Filename", a.FileName)
		aresp, _, err := glc.Projects.UploadFile(p.Pid, a.FileName, nil, nil)
		if err != nil {
			contextLogger.Error(err)
			break
		}

		contextLogger.Info("file uploaded")
		gin := gitlab.CreateIssueNoteOptions{Body: &aresp.Markdown}
		// create a note with the attachement file .
		_, _, err = glc.Notes.CreateIssueNote(p.Pid, o.IID, &gin)

		if err != nil {
			contextLogger.Error(err)
			break
		}

		// lets clean-up after ourselves
		err = os.Remove(a.FileName)
		if err != nil {
			contextLogger.WithError(err).Errorln("unable to remove attachement file")
		}
		contextLogger.Info("created")
	}
	// status closed ?? np ... we got ya

	cs := "close"

	if i.Status != "Open" {
		contextLogger.Info("attempting to recreate status closed in gitlab")
		_, _, err := glc.Issues.UpdateIssue(p.Pid, o.IID, &gitlab.UpdateIssueOptions{StateEvent: &cs}, nil)
		if err != nil {
			contextLogger.Errorf("unable to close issue: %s", err)
		} else {
			contextLogger.Info("issue closed")
		}

	}
	return nil
}

func translateText(t string) string {
	translations := map[string]string{
		"[noformat]":  "```",
		"{noformat}":  "```",
		"{code:java}": "```java",
		"{code:ruby}": "```ruby",
		"{code}":      "```",
	}

	for o, r := range translations {
		t = strings.Replace(t, o, r, -1)
	}

	return t
}
