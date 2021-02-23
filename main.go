package main

// AAAAAA
import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"path"
	"time"

	"os"
	"path/filepath"
	"regexp"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/cosiner/flag"
	"github.com/rwcarlsen/goexif/exif"
)

type photoOpts struct {
	MoveFiles    bool     `name:"-move" usage:"Move instead of copy"`
	IgnoreExif   bool     `name:"-ignore-exif" usage:"Ignore EXIF data"`
	Recursive    bool     `name:"-recursive" usage:"Recurse into subdirectories in source"`
	Verbose      bool     `name:"-verbose" usage:"Print messages to log" default:"False"`
	ImagePattern string   `name:"-filter" usage:"Regex describing filenames" default:"(?i)\\.(jpg|dng|tiff|jpeg|mpg|mp4|mov)$"`
	Paths        []string `args:"true"`

	// non-option, derived values
	// These values are used for the processing.
	imageRegexp regexp.Regexp // holds our regular expression
	sourceDirs  []string      // source paths
	destDir     string        // target directory
}

func copyFile(src, dest string) error {
	// copies a file from the source to the destination, preserving as much as go will let us.

	var srcfd *os.File
	var dstfd *os.File
	var err error
	srcinfo, _ := os.Stat(src)

	if srcinfo.IsDir() {
		return errors.New("Not a file: " + src)
	}

	srcfd, err = os.Open(src)
	if err != nil {
		return err
	}
	dstfd, err = os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, srcinfo.Mode().Perm())
	if err != nil {
		return err
	}
	written, err := io.Copy(dstfd, srcfd)
	if err != nil {
		return err
	} else if written < srcinfo.Size() {
		return errors.New(fmt.Sprintf("Failed to write full file. Wrote %v of %v bytes", written, srcinfo.Size()))
	}
	srcfd.Close()
	dstfd.Close()
	// Set some minor things about the file

	os.Chtimes(dest, time.Now(),srcinfo.ModTime())
	return nil

}

func (P *photoOpts) Metadata() map[string]flag.Flag {
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

func processDirectory(sourceDir string, opts photoOpts) error {

	log.Trace().Str("path", sourceDir).Msg("enter processDirectory")
	// Check the path to validate that it's a folder.
	stat, err := os.Stat(sourceDir)
	if err != nil {
		// Something went wrong, bubble up the error!
		return err
	} else if stat.IsDir() == false {
		log.Fatal().
			Str("path", sourceDir).
			Msg("Path is not a directory.")
	}
	items, err := ioutil.ReadDir(sourceDir)
	for _, file := range items {
		fullPathName := filepath.Join(sourceDir, file.Name())
		fileIsPhoto := opts.imageRegexp.MatchString(file.Name())
		if fileIsPhoto == false {
			log.Trace().
				Str("filename", fullPathName).
				Msg("Skipping: didn't pass regex.")
			continue
		}

		photoTime := file.ModTime()
		if opts.IgnoreExif == false {

			photoFileReader, err := os.Open(fullPathName)
			if err != nil {
				log.Fatal().
					Err(err).
					Str("filename", file.Name()).
					Msg("Failed to open file on disk!")
			}
			defer photoFileReader.Close()
			decoder, err := exif.Decode(photoFileReader)
			if err == nil {
				// Use the decoder's dateTime
				exifTime, err := decoder.DateTime()
				if err == nil {
					log.Trace().Msg("EXIF data loaded!")
					photoTime = exifTime
				} else {
					log.Debug().Err(err).
						Str("filename", fullPathName).
						Msg("Failed to load EXIF date/time from EXIF source")
				}
			}
		}
		log.Debug().
			Str("filename", fullPathName).
			Time("photoDate", photoTime).
			Msg("Loaded photo & ready to put in destination")

		// Send this file to the right place.
		destinationDirectory := path.Join(opts.destDir, photoTime.Format("2006-01"))
		destinationPathFull :=  path.Join(destinationDirectory, file.Name())
		log.Info().
			Str("sourceFile", fullPathName).
			Str("destFile", destinationPathFull).
			Msg("Copying file to destination")
		// Make sure the destination directory exists
		err := os.MkdirAll(destinationDirectory, 660)
		if err != nil {
			log.Fatal().Str("path", destinationDirectory).Err(err).Msg("Failed to create final directory")
		}
		err = copyFile(fullPathName, destinationPathFull)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to copy file")
		}

	}
	// If at this point we had no issue, return
	return nil
}

func main() {
	var opts photoOpts

	// Log setup

	// parse command line arguments
	set := flag.NewFlagSet(flag.Flag{})
	set.NeedHelpFlag(true)
	set.StructFlags(&opts)
	set.Parse()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if opts.Verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	// build the regular expression that is used
	matchRegex, err := regexp.Compile(opts.ImagePattern)
	opts.imageRegexp = *matchRegex

	if err != nil {
		log.Fatal().Err(err).Str("regex", opts.ImagePattern).Msg("Failed to parse file match regular expression")
	}

	// Check to make sure that there are enough sources
	if len(opts.Paths) < 2 {
		log.Fatal().Msg("Expected 2+ paths, got 1 or less!")
	}

	// The first part of the paths set is the sources.
	opts.sourceDirs = opts.Paths[:len(opts.Paths)-1]
	// The last element is the destination.
	opts.destDir = opts.Paths[len(opts.Paths)-1]
	// Check that the paths exist.
	for _, sourceDir := range opts.sourceDirs {
		processDirectory(sourceDir, opts)
	}

}
