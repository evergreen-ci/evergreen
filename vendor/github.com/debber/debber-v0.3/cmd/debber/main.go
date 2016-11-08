package main

import (
	"log"
	"os"
)

const (
	TaskInit              = "init"
	TaskChangelogAdd      = "changelog:add"
	TaskGenDeb            = "deb:gen"
	TaskDebControl        = "deb:control"
	TaskDebContents       = "deb:contents"
	TaskDebContentsDebian = "deb:contents-debian"
	TaskGenDev            = "dev:gen"
	TaskGenSource         = "source:gen"
	TaskControlInit       = "control:init"
	TaskRulesInit         = "rules:init"
	TaskCopyrightInit     = "copyright:init"
)

var tasks = []string{
	TaskInit,
	TaskChangelogAdd,
	TaskGenDeb,
	TaskGenSource,
	TaskDebControl,
	TaskDebContents,
	TaskDebContentsDebian,
	TaskControlInit,
	TaskRulesInit,
	TaskCopyrightInit,
}

func main() {
	name := "debber"
	log.SetPrefix("[" + name + "] ")
	if len(os.Args) < 2 {
		log.Printf("Please specify a task (one of: %v)", tasks)
		log.Printf("For help on any task, use `debber <task> -h`")
		os.Exit(1)
	}
	task := os.Args[1]
	args := os.Args[2:]
	var err error
	switch task {
	case TaskInit:
		err = initDebber(args)
	case TaskChangelogAdd:
		err = changelogAddEntryTask(args)
	case TaskControlInit:
		err = controlInitTask(args)
	case TaskRulesInit:
		err = rulesInitTask(args)
	case TaskCopyrightInit:
		err = copyrightInitTask(args)
	case TaskGenDeb:
		err = debGen(args)
	case TaskGenSource:
		err = sourceGen(args)
	case TaskDebControl:
		debControl(args)
	case TaskDebContents:
		debContents(args)
	case TaskDebContentsDebian:
		debContentsDebian(args)
	case "help", "h", "-help", "--help", "-h":
		log.Printf("Please specify one of: %v", tasks)
		log.Printf("For help on any task, use `debber <task> -h`")
	default:
		log.Printf("Unrecognised task '%s'", task)
		log.Printf("Please specify one of: %v", tasks)
		log.Printf("For help on any task, use `debber <task> -h`")
		os.Exit(1)
	}
	if err != nil {
		log.Printf("%v", err)
		os.Exit(1)
	}

}
