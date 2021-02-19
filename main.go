package main

// AAAAAA
import (
	"fmt"
	"log"
	"os"

	"github.com/cosiner/flag"
)

type PhotoOpts struct {
	MoveFiles    bool     `name:"-move" usage:"Move instead of copy"`
	IgnoreExif   bool     `name:"-ignore-exif" usage:"Ignore EXIF data"`
	Verbose      bool     `name:"-verbose" usage:"Print Verbose Output"`
	ImagePattern string   `name:"-filter" usage:"Regex describing filenames" default:"\\.(jpg|dng|tiff|jpeg)"`
	Paths        []string `args:"true"`
}

func (P *PhotoOpts) Metadata() map[string]flag.Flag {
	const (
		usage   = "PhotoMunch ingests photos based on their EXIF data"
		version = "v1"
		desc    = `
		PhotoMunch reads the EXIF data (or, barring that, file modification date) for all files in the
		source directory that match the filter and copies (or moves) the files into the destination directory
		based on a target template path.
		`
	)
	return map[string]flag.Flag{
		"": {
			Usage:   usage,
			Version: version,
			Desc:    desc,
		},
	}
}

func main() {
	var opts PhotoOpts

	// Log setup
	log.SetPrefix("PhotoMonger: ")

	// parse command line arguments
	set := flag.NewFlagSet(flag.Flag{})
	set.NeedHelpFlag(true)
	set.StructFlags(&opts)
	set.Parse()

	// Check to make sure that there are enough sources
	if len(opts.Paths) < 2 {
		log.Fatal("Not enough arguments (expected 2)")
	}

	// The first part of the paths set is the sources.
	sourceDirs := opts.Paths[:len(opts.Paths)-1]
	// The last element is the destination.
	destDir := opts.Paths[len(opts.Paths)-1]
	// Check that the paths exist.
	for _, sourceDir := range sourceDirs {
		stat, err := os.Stat(sourceDir)
		if err != nil {
			log.Fatal(fmt.Sprintf("Path %s does not exist", sourceDir))
		} else if stat.IsDir() == false {
			log.Fatal(fmt.Sprintf("Path %s is not a file", sourceDir))
		}
		log.Printf("Source: %s ", sourceDir)
	}
	log.Printf("Destination: %s", destDir)

}
