package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	jira "github.com/wianvos/go-jira"
	gitlab "github.com/xanzy/go-gitlab"
)

var GitlabClient *gitlab.Client
var JiraClient *jira.Client

//GetJiraClient returns a Jira client object and checks the validity of the connections
func GetJiraClient() *jira.Client {
	if JiraClient != nil {
		return JiraClient
	}

	jiraURL := viper.GetString("jiraUrl")
	jiraAccountUsername := viper.GetString("jiraAccountUsername")
	jiraAccountPassword := viper.GetString("jiraAccountPassword")

	log.Infof("connecting to jira: %s using %s ", jiraURL, jiraAccountUsername)

	tp := jira.BasicAuthTransport{
		Username: jiraAccountUsername,
		Password: jiraAccountPassword,
	}
	// tr := &http.Transport{
	// 	TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	// }

	client, err := jira.NewClient(tp.Client(), jiraURL)
	if err != nil {
		fmt.Println(err)
	}

	if client.Authentication.Authenticated() {
		log.Fatal("unabel to connect to jira")
		fmt.Println("unable to connect to jira")
		os.Exit(2)
	}
	log.Info("connected to jira.. ready to rock")
	JiraClient = client
	return client
}

//GetGitlabClient returns a gitlab client object
func GetGitlabClient() *gitlab.Client {
	if GitlabClient != nil {
		return GitlabClient
	}

	gitlabURL := viper.GetString("gitlabURL")
	gitlabToken := viper.GetString("gitlabToken")

	log.Infof("connecting to gitlab: %s using token ", gitlabURL)

	glc := gitlab.NewClient(nil, gitlabToken)
	glc.SetBaseURL(gitlabURL)
	GitlabClient = glc
	return glc
}

func GetTmpDir() string {
	tmpDir := viper.GetString("localTmpDir")
	t := time.Now().Format(time.RFC3339)
	d := fmt.Sprintf("%s/%s", tmpDir, t)
	if _, err := os.Stat(tmpDir); err == nil {
		log.Debugf("creating temporary directory %s", tmpDir)
		os.Mkdir(d, 0770)
	} else {
		log.Debugf("creating temporary directory %s", tmpDir)
		os.MkdirAll(d, 0770)
	}

	return d
}

func GetTmpFileName() string {
	return fmt.Sprintf("%s/%s", GetTmpDir(), uuid.NewV4())
}

func GetTmpDirFileName(fn string) string {
	return fmt.Sprintf("%s/%s", GetTmpDir(), fn)
}

//WriteToFile writes any string output to file
func WriteToFile(s string, f string) {
	d1 := []byte(s + "\n")
	err := ioutil.WriteFile(f, d1, 0644)
	if err != nil {
		panic(err)
	}
}

//RenderJSON ... renders json :-) from an interfaces .. as a string..
// and all that in just 6 lines of code ..
func RenderJSON(l interface{}) string {

	b, err := json.MarshalIndent(l, "", " ")
	if err != nil {
		panic(err)
	}
	s := string(b)

	return s
}

// viper.BindPFlag("jiraURL", RootCmd.PersistentFlags().Lookup("jiraURL"))
// 	viper.BindPFlag("jiraAccountPassword", RootCmd.PersistentFlags().Lookup("jiraAccountPassword"))
// 	viper.BindPFlag("jiraAccountUsername", RootCmd.PersistentFlags().Lookup("jiraAccountUsername"))
// 	viper.BindPFlag("jiraProject", RootCmd.PersistentFlags().Lookup("jiraProject"))
// 	viper.BindPFlag("gitlabURL", RootCmd.PersistentFlags().Lookup("gitlabURL"))
// 	viper.BindPFlag("gitlabToken", RootCmd.PersistentFlags().Lookup("gitlabToken"))
// 	viper.BindPFlag("gitlabProjectID", RootCmd.PersistentFlags().Lookup("gitlabProjectID"))
// 	viper.BindPFlag("logLevel", RootCmd.PersistentFlags().Lookup("logLevel"))
// 	viper.BindPFlag("logFile", RootCmd.PersistentFlags().Lookup("logFile"))
// 	viper.BindPFlag("logToFile", RootCmd.PersistentFlags().Lookup("logToFile"))
