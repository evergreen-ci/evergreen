package main

import (
	"errors"
	"fmt"
	"github.com/jessevdk/go-flags"
	"os"
	"strconv"
	"strings"
)

type EditorOptions struct {
	Input  flags.Filename `short:"i" long:"input" description:"Input file" default:"-"`
	Output flags.Filename `short:"o" long:"output" description:"Output file" default:"-"`
}

type Point struct {
	X, Y int
}

func (p *Point) UnmarshalFlag(value string) error {
	parts := strings.Split(value, ",")

	if len(parts) != 2 {
		return errors.New("expected two numbers separated by a ,")
	}

	x, err := strconv.ParseInt(parts[0], 10, 32)

	if err != nil {
		return err
	}

	y, err := strconv.ParseInt(parts[1], 10, 32)

	if err != nil {
		return err
	}

	p.X = int(x)
	p.Y = int(y)

	return nil
}

func (p Point) MarshalFlag() (string, error) {
	return fmt.Sprintf("%d,%d", p.X, p.Y), nil
}

type Options struct {
	// Example of verbosity with level
	Verbose []bool `short:"v" long:"verbose" description:"Verbose output"`

	// Example of optional value
	User string `short:"u" long:"user" description:"User name" optional:"yes" optional-value:"pancake"`

	// Example of map with multiple default values
	Users map[string]string `long:"users" description:"User e-mail map" default:"system:system@example.org" default:"admin:admin@example.org"`

	// Example of option group
	Editor EditorOptions `group:"Editor Options"`

	// Example of custom type Marshal/Unmarshal
	Point Point `long:"point" description:"A x,y point" default:"1,2"`
}

var options Options

var parser = flags.NewParser(&options, flags.Default)

type BAddCommand struct {
	All bool `short:"a" long:"all" description:"Add all files"`
}

func (x *BAddCommand) Execute(args []string) error {
	fmt.Printf("Adding (all=%v): %#v\n", x.All, args)
	return nil
}

func init() {
	fmt.Println("adding parser command")
}

func main() {
	var baddCommand BAddCommand
	parser.AddCommand("badd",
		"bAdd a file",
		"The badd command adds a file to the repository. Use -a to add all files.",
		&baddCommand)
	x, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
	fmt.Println(x)
}
