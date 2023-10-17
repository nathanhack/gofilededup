package cmd

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var inputDir []string
var dupDump string

type PathTime struct {
	Path string
	Time time.Time
}

var files = map[string]PathTime{}
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
	Run: func(cmd *cobra.Command, args []string) {
		filepath.Walk(args[0], func(path string, info os.FileInfo, e error) error {
			if e != nil {
				logrus.Error(e)
				return e
			}

			if info.Mode().IsDir() {
				return nil
			}
			logrus.Info(path)
			if info.Size() == 0 {
				return nil
			}

			f, err := os.Open(path)
			if err != nil {
				logrus.Error(err)
				return err
			}
			defer f.Close()

			h := sha256.New()
			if _, err := io.Copy(h, f); err != nil {
				logrus.Fatal(err)
			}

			sha := fmt.Sprintf("%x", h.Sum(nil))

			old, has := files[sha]

			if !has {
				files[sha] = PathTime{path, info.ModTime()}
				return nil
			}

			if old.Time.After(info.ModTime()) || len(old.Path) > len(path) {
				files[sha] = PathTime{path, info.ModTime()}
				return moveToDump(old.Path)
			}

			return moveToDump(path)
		})
	},
}

func moveToDump(filename string) error {

	full := filepath.Join(dupDump, filename)

	if dryrun {
		logrus.Warnf("Moving %v to %v", filename, full)
		return nil
	}

	err := os.MkdirAll(filepath.Dir(full), 0755)
	if err != nil {
		logrus.Error(err)
		return err
	}
	logrus.Warnf("Moving %v to %v", filename, full)
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
	// rootCmd.Flags().StringSliceVarP(&inputDir, "inputDir", "i", []string{}, "Directories to scan.")
	// rootCmd.MarkFlagRequired("inputDir")
	// rootCmd.MarkFlagDirname("inputDir")
	rootCmd.Flags().BoolVar(&dryrun, "dryrun", false, "Sets to do a dryrun before running for real")
	rootCmd.Flags().StringVarP(&dupDump, "dups", "d", "./dupdump", "Directory to dump dups into.")
	rootCmd.MarkFlagDirname("dups")
}
