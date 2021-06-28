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

	// hack: A logger reference
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
	// Open the source file handle. This is a read-write (though, we could make it RO)
	srcfd, err = os.Open(src)
	if err != nil {
		srcfd.Close()
		return err
	}
	// Open the destination file handle. This is a write-only file descriptor.
	dstfd, err = os.Create(dest)
	if err != nil {
		srcfd.Close()
		dstfd.Close()
		return err
	}

	// Copy Do the actual copy.
	written, err := io.Copy(dstfd, srcfd)
	// And close them
	srcfd.Close()
	dstfd.Close()

	// There are two ways this can fail:
	// * The copy call can fail (duh)
	// * The final number of bytes written can be unequal to the size of the source file.
	if err != nil {
		return err
	} else if written != srcinfo.Size() {
		return errors.New(fmt.Sprintf("Failed to write full file. Wrote %v of %v bytes", written, srcinfo.Size()))
	}
	// Set some minor things about the file

	os.Chtimes(dest, time.Now(), srcinfo.ModTime())
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

func processDirectory(sourceDir string, opts photoOpts, logger *zerolog.Logger) error {

	logger.Trace().Str("path", sourceDir).Msg("enter processDirectory")
	// Check the path to validate that it's a folder.
	stat, err := os.Stat(sourceDir)
	if err != nil {
		// Something went wrong, bubble up the error!
		return err
	} else if stat.IsDir() == false {
		logger.Fatal().
			Str("path", sourceDir).
			Msg("Path is not a directory.")
	}
	items, err := ioutil.ReadDir(sourceDir)
	if err != nil {
		logger.Fatal().Str("path", sourceDir).Err(err).Msg("Failed to read directory")
	}
	for _, file := range items {
		fullPathName := filepath.Join(sourceDir, file.Name())
		// If it's a directory, we can go process it.
		if file.IsDir() {
			if opts.Recursive {
				processDirectory(fullPathName, opts, logger)
			}
			continue
		}

		fileIsPhoto := opts.imageRegexp.MatchString(file.Name())
		if !fileIsPhoto {
			logger.Trace().
				Str("filename", fullPathName).
				Msg("Skipping: didn't pass regex.")
			continue
		}

		photoTime := file.ModTime()
		if !opts.IgnoreExif {

			photoFileReader, err := os.Open(fullPathName)
			if err != nil {
				logger.Fatal().
					Err(err).
					Str("filename", file.Name()).
					Msg("Failed to open file on disk!")
			}
			decoder, err := exif.Decode(photoFileReader)
			if err == nil {
				// Use the decoder's dateTime
				exifTime, err := decoder.DateTime()
				if err == nil {
					logger.Trace().Msg("EXIF data loaded!")
					photoTime = exifTime
				} else {
					logger.Debug().Err(err).
						Str("filename", fullPathName).
						Msg("Failed to load EXIF date/time from EXIF source")
				}
			}
			photoFileReader.Close()

		}
		logger.Debug().
			Str("filename", fullPathName).
			Time("photoDate", photoTime).
			Msg("Loaded photo & ready to put in destination")

		// Send this file to the right place.
		destinationDirectory := path.Join(opts.destDir, photoTime.Format("2006-01"))
		destinationPathFull := path.Join(destinationDirectory, file.Name())
		logger.Info().
			Str("sourceFile", fullPathName).
			Str("destFile", destinationPathFull).
			Msg("Copying file to destination")
		// Make sure the destination directory exists
		err := os.MkdirAll(destinationDirectory, 0760)
		if err != nil {
			logger.Fatal().Str("path", destinationDirectory).Err(err).Msg("Failed to create final directory")
		}
		if opts.MoveFiles {
			err = os.Rename(fullPathName, destinationPathFull)
			if err != nil {
				logger.Error().
					Err(err).
					Str("sourceFile", fullPathName).
					Str("destFile", destinationPathFull).
					Msg("Failed to move target file to destination")
			}
		}
		err = copyFile(fullPathName, destinationPathFull)
		if err != nil {
			logger.Fatal().Err(err).Msg("Failed to copy file")
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
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
		log.Trace().Msg("TRACING ENABLED")
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
	log.Trace().Strs("paths", opts.Paths).Msg("Start processing")
	// The first part of the paths set is the sources.
	opts.sourceDirs = opts.Paths[:len(opts.Paths)-1]
	// The last element is the destination.
	opts.destDir = opts.Paths[len(opts.Paths)-1]
	// Check that the paths exist.
	for _, sourceDir := range opts.sourceDirs {
		log.Trace().Str("sourceDir", sourceDir).Msg("start processDirectory")
		processDirectory(sourceDir, opts, &log.Logger)
	}

}
