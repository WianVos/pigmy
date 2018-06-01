package migrate

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var migrateCMD = &cobra.Command{
	Use:   "migrate",
	Short: "migrate jira to gitlab project stuff",
}

var contextLogger = log.WithFields(log.Fields{"Command": "Migrate"})

//GetCommands grab and return commands in this package
func GetCommands() *cobra.Command {

	//collect the commands in the package
	addProject()
	return migrateCMD
}
