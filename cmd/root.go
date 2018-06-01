// Copyright © 2017 Roy Kliment <roy.kliment@cinqict.nl>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/onrik/logrus/filename"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wianvos/pigmy/cmd/migrate"
	gitlab "github.com/xanzy/go-gitlab"
)

// vars for app
var cfgFile string
var verbose bool

// JIRA_URL = 'https://your-jira-url.tld/'
// JIRA_ACCOUNT = ('jira-username', 'jira-password')
// # the JIRA project ID (short)
// JIRA_PROJECT = 'PRO'
// GITLAB_URL = 'http://your-gitlab-url.tld/'
// # this token will be used whenever the API is invoked and
// # the script will be unable to match the jira's author of the comment / attachment / issue
// # this identity will be used instead.
// GITLAB_TOKEN = 'get-this-token-from-your-profile'
// # the project in gitlab that you are importing issues to.
// GITLAB_PROJECT = 'namespaced/project/name'
// # the numeric project ID. If you don't know it, the script will search for it
// # based on the project name.
// GITLAB_PROJECT_ID = None
// # set this to false if JIRA / Gitlab is using self-signed certificate.
// VERIFY_SSL_CERTIFICATE = True

var jiraURL string
var jiraAccountUsername string
var jiraAccountPassword string
var jiraProject string
var gitlabURL string
var gitlabToken string
var gitlabProjectID string
var localTmpDir string
var logLevel string
var logFile string

var logToFile bool

var projectMembers []*gitlab.ProjectMember

const chunksize = 250
const dummypw = "haveaniceday"

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:              "pigmy",
	Short:            "migrate jira issues to gitlab project issues",
	PersistentPreRun: initializeLogging,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {

	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {

	log.SetFormatter(&log.JSONFormatter{})

	// Only log the warning severity or above.

	cobra.OnInitialize(initConfig, processConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	RootCmd.PersistentFlags().StringVar(&jiraURL, "jiraURL", "", "jira server url")
	RootCmd.PersistentFlags().StringVar(&jiraAccountPassword, "jiraAccountPassword", "", "jira server password")
	RootCmd.PersistentFlags().StringVar(&jiraAccountUsername, "jiraAccountUsername", "", "jira server username")
	RootCmd.PersistentFlags().StringVar(&jiraProject, "jiraProject", "", "jira project to copy issues from")
	RootCmd.PersistentFlags().StringVar(&gitlabURL, "gitlabURL", "", "gitlab server URL")
	RootCmd.PersistentFlags().StringVar(&gitlabToken, "gitlabToken", "", "gitlab access token")
	RootCmd.PersistentFlags().StringVar(&gitlabProjectID, "gitlabProjectID", "", "gitlab project id")
	RootCmd.PersistentFlags().StringVar(&localTmpDir, "localTmpDir", "./tmp", "temporary file dir")
	RootCmd.PersistentFlags().BoolVar(&logToFile, "logToFile", true, "log to file?")
	RootCmd.PersistentFlags().StringVar(&logLevel, "logLevel", "warning", "set pigmy loglevel")
	RootCmd.PersistentFlags().StringVar(&logFile, "logFile", "./pigmy.log", "set pigmy logfile")

	viper.BindPFlag("jiraURL", RootCmd.PersistentFlags().Lookup("jiraURL"))
	viper.BindPFlag("jiraAccountPassword", RootCmd.PersistentFlags().Lookup("jiraAccountPassword"))
	viper.BindPFlag("jiraAccountUsername", RootCmd.PersistentFlags().Lookup("jiraAccountUsername"))
	viper.BindPFlag("jiraProject", RootCmd.PersistentFlags().Lookup("jiraProject"))
	viper.BindPFlag("gitlabURL", RootCmd.PersistentFlags().Lookup("gitlabURL"))
	viper.BindPFlag("gitlabToken", RootCmd.PersistentFlags().Lookup("gitlabToken"))
	viper.BindPFlag("gitlabProjectID", RootCmd.PersistentFlags().Lookup("gitlabProjectID"))
	viper.BindPFlag("logLevel", RootCmd.PersistentFlags().Lookup("logLevel"))
	viper.BindPFlag("logFile", RootCmd.PersistentFlags().Lookup("logFile"))
	viper.BindPFlag("logToFile", RootCmd.PersistentFlags().Lookup("logToFile"))
	viper.BindPFlag("localTmpDir", RootCmd.PersistentFlags().Lookup("localTmpDir"))

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// RootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	//add subcommand object to the root command
	RootCmd.AddCommand(migrate.GetCommands())

}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
		viper.SetConfigType("json")
	} else {

		// Search config in current directory with name "xldc" (without extension).
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/pigmy")

		viper.AddConfigPath("$HOME/.pigmy/")
		viper.SetConfigName("pigmy")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Printf("Using config file: %s \n", viper.ConfigFileUsed())
	} else {
		fmt.Print(err)
		fmt.Printf("Config file: %s not found. No configuration provided... exiting", viper.ConfigFileUsed())
		os.Exit(0)
	}
}

// processConfig will use viper config if flag is not set
func processConfig() {
	if jiraAccountUsername == "" && viper.IsSet("jiraAccountUsername") {
		jiraAccountUsername = viper.GetString("jiraAccountUsername")
	}
	if jiraAccountPassword == "" && viper.IsSet("jiraAccountPassword") {
		jiraAccountPassword = viper.GetString("jiraAccountPassword")
	}
	if jiraURL == "" && viper.IsSet("jiraURL") {
		jiraURL = viper.GetString("jiraURL")
	}
	if jiraProject == "" && viper.IsSet("jiraProject") {
		jiraProject = viper.GetString("jiraProject")
	}
	if gitlabURL == "" && viper.IsSet("gitlabURL") {
		gitlabURL = viper.GetString("gitlabURL")
	}
	if gitlabToken == "" && viper.IsSet("gitlabToken") {
		gitlabToken = viper.GetString("gitlabToken")
	}
	if gitlabProjectID == "" && viper.IsSet("gitlabProjectID") {
		gitlabProjectID = viper.GetString("gitlabProjectID")
	}
	if localTmpDir == "" && viper.IsSet("localTmpDir") {
		localTmpDir = viper.GetString("localTmpDir")
	}

}
func initializeLogging(cmd *cobra.Command, args []string) {

	// log to file or stdout
	if logToFile {
		logFile = fmt.Sprintf("%s.%s", logFile, time.Now().Format(time.RFC3339))
		// log.Infof("logging to file: %s", logFile)

		file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY, 0666)
		if err == nil {

			log.SetOutput(file)
			// log.Infof("logging to logfile: %s", logFile)

		} else {
			log.SetOutput(os.Stdout)
			// log.Info("Failed to log to file, using default stdout")
		}

	} else {
		//logging to Stdout
		log.SetOutput(os.Stdout)
	}

	// parse the logLevel parameter to a valid logrus loglevel
	l, e := log.ParseLevel(logLevel)

	if e != nil {

		// log.Error("unable to parse logLevel parameter. Defaulting to warning")
		log.SetLevel(log.WarnLevel)

	} else {

		log.SetLevel(l)
		// log.Infof("setting loglevel to: %s", l.String())

	}

	// set a hook in logrus revealing where a certain log message came from.. god i love logrus
	filenameHook := filename.NewHook()
	filenameHook.Field = "source" // Customize source field name
	log.AddHook(filenameHook)

}
