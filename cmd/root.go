package cmd

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var ddir string
var dedup bool
var rdup bool
var remove bool
var fdir string
var flatten bool

type PathTime struct {
	Path string
	Time time.Time
}

var files = map[string]PathTime{}
var dupFiles = map[PathTime]bool{}
var dryrun bool
var rootCmd = &cobra.Command{
	Use:   "gofilededup INPUT_DIR",
	Short: "Commandline tool to dedup files.",
	Long: `Commandline tool to dedup files.
		When dups are found the oldest and shortest name wins.
		Dups are moved to the dupDump directory.
		Empty files are skipped.
	`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("requires the path to the input directory to deduplicate files")
		}

		if _, err := os.Stat(args[0]); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("input directory to deduplicate file must exist")
		}

		if flatten {
			if _, err := os.Stat(fdir); !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("flatten directory must not exist")
			}
		}

		err := filepath.Walk(args[0], func(path string, info os.FileInfo, e error) error {
			if e != nil {
				logrus.Error(e)
				return e
			}

			if info.Mode().IsDir() {
				return nil
			}

			if info.Size() == 0 {
				logrus.Infof("Found: %v : SKIPPING filesize:0", path)
				return nil
			}

			// for each file we open and run sha256 on it
			f, err := os.Open(path)
			if err != nil {
				logrus.Error(err)
				return err
			}
			defer f.Close()

			h := sha256.New()
			if _, err := io.Copy(h, f); err != nil {
				logrus.Fatal(err)
				return nil
			}

			sha := fmt.Sprintf("%x", h.Sum(nil))
			logrus.Infof("Found: %v : %v", path, sha)
			// now we keep a history so we check if it's already in the history
			// if not we add it
			// and if it does exist we do some checks to decide which file will be the "duplicate"

			old, has := files[sha]

			fileInfo := PathTime{path, info.ModTime()}
			if !has {
				files[sha] = fileInfo
				return nil
			}

			if old.Time.After(info.ModTime()) || len(old.Path) > len(path) {
				delete(dupFiles, files[sha])
				files[sha] = fileInfo

			}
			dupFiles[fileInfo] = true

			return nil
		})

		if err != nil {
			return err
		}

		if dedup {
			if rdup {
				logrus.Infof("Duplicate files will be moved to %v", ddir)
				for file := range dupFiles {
					moveToDirectory(file.Path, ddir, file.Path)
				}
			} else {
				logrus.Infof("Duplicate files will be copied to %v", ddir)
				for file := range dupFiles {
					copyToDirectory(file.Path, ddir, file.Path)
				}
			}
		} else if rdup {
			logrus.Infof("Duplicate files will be removed from %v", args[0])
			for file := range dupFiles {
				filename := filepath.Join(filepath.Dir(filepath.Clean(args[0])), file.Path)
				logrus.Warnf("Removing %v", filename)
				if !dryrun {
					err := os.Remove(filename)
					if err != nil {
						return err
					}
				}
			}
		}

		filenames := make(map[string]int)
		if flatten {
			logrus.Infof("Non duplicate files will be flatten in %v", fdir)
			for _, file := range files {

				// so at this point we have unique files but the names
				// could be duplicated so we'll make them unique
				flattenFilename := filepath.Base(file.Path)
				if x, has := filenames[flattenFilename]; has {
					flattenFilename = fmt.Sprint(flattenFilename, x)
					filenames[flattenFilename]++
				} else {
					filenames[flattenFilename] = 1
				}

				if remove {
					moveToDirectory(file.Path, fdir, flattenFilename)
				} else {
					copyToDirectory(file.Path, fdir, flattenFilename)
				}
			}
		}
		return nil
	},
}

func copyToDirectory(filename string, destinationDir string, newFilename string) error {
	full := filepath.Join(destinationDir, filename)
	if newFilename != "" {
		full = filepath.Join(destinationDir, newFilename)
	}

	logrus.Warnf("Copying %v to %v", filename, full)
	if dryrun {
		return nil
	}

	err := os.MkdirAll(filepath.Dir(full), 0755)
	if err != nil {
		logrus.Error(err)
		return err
	}

	in, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(full)
	if err != nil {
		return err
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func moveToDirectory(filename string, destinationDir string, newFilename string) error {

	full := filepath.Join(destinationDir, filename)
	if newFilename != "" {
		full = filepath.Join(destinationDir, newFilename)
	}

	logrus.Warnf("Moving %v to %v", filename, full)
	if dryrun {
		return nil
	}

	err := os.MkdirAll(filepath.Dir(full), 0755)
	if err != nil {
		logrus.Error(err)
		return err
	}

	err = os.Rename(filename, full)
	if err != nil {
		logrus.Error(err)
		return err
	}
	return nil
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVar(&dryrun, "dryrun", false, "Sets to do a dryrun before running for real")

	rootCmd.Flags().StringVar(&ddir, "ddir", "./dupdump", "Directory to copy duplicate files into, it will retain the relative filepath.")
	rootCmd.MarkFlagDirname("ddir")
	rootCmd.Flags().BoolVar(&dedup, "dedup", false, "Enable saving a copy of the duplicates to the --ddir directory.")
	rootCmd.Flags().BoolVar(&rdup, "rdup", false, "When enabled all duplicate files in input directory will be removed.")

	rootCmd.Flags().StringVar(&fdir, "fdir", "./flatten", "Directory to copy all files with flattened relative directories into.")
	rootCmd.MarkFlagDirname("fdir")
	rootCmd.Flags().BoolVar(&flatten, "flatten", false, "Enable saving off the all non duplicated files to the --fdir directory.")
	rootCmd.Flags().BoolVar(&remove, "remove", false, "When enabled all non-duplicate files in input directory will be removed.")

}
