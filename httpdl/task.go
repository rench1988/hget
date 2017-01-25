package httpdl

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

const (
	dataFolder = ".hget/"
)

func DlTaskPrint() error {
	downloading, err := ioutil.ReadDir(filepath.Join(os.Getenv("HOME"), dataFolder))
	if err != nil {
		return errors.New("Failed read directory: " + err.Error())
	}

	folders := make([]string, 0)
	for _, d := range downloading {
		if d.IsDir() {
			folders = append(folders, d.Name())
		}
	}

	folderString := strings.Join(folders, "\n")
	fmt.Printf("Currently on going download: \n")
	fmt.Println(folderString)

	return nil
}

func DlTaskResume(url string) error {
	stateFile, err := filepath.Abs(filepath.Join(os.Getenv("HOME"), dataFolder, filepath.Base(url)+".status"))
	if err != nil {
		return errors.New("failed get state file name: " + err.Error())
	}

	bytes, err := ioutil.ReadFile(stateFile)
	if err != nil {
		return errors.New("failed read state file name: " + err.Error())
	}

	dl := new(Httpdl)

	err = json.Unmarshal(bytes, dl)
	if err != nil {
		return errors.New("failed unmarshal json data in state file: " + err.Error())
	}

	dl.dlRepart()

	dl.connsem = make(chan bool, dl.Maxconn)
	dl.errs = make(chan error, len(dl.Parts))
	dl.resumable = true
	dl.skipTls = true

	if err = dlFile(dl); err != nil {
		return err
	}

	fmt.Println(dl)

	return dl.Do()
}

func DlTaskDo(dl *Httpdl, url string, rangeSize uint, connNum int, skiptls bool) error {
	var (
		dltask *Httpdl
		err    error
	)

	if dl == nil {
		dltask, err = New(url, rangeSize, connNum, skiptls)
		if err != nil {
			return errors.New("failed create http download task: " + err.Error())
		}
	}

	return dltask.Do()
}
