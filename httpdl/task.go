package httpdl

import (
    "io/ioutil"
    "path/filepath"
    "os"
    "fmt"
    "strings"
)

const (
    dataFolder = ".hget/"
)



func DlTaskPrint() {
	downloading, err := ioutil.ReadDir(filepath.Join(os.Getenv("HOME"), dataFolder))
	if err != nil {
		fmt.Println("Failed read directory", err)
        return
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

	return     
}

func DlTaskResume(url string) {
    stateFile, err := filepath.Abs(filepath.Join(os.Getenv("HOME"), dataFolder, filepath.Base(url) + ".status"))
    if err != nil {
        fmt.Println("failed get state file name", err)
        return
    }

    bytes, err := ioutil.ReadFile(stateFile)
	if err != nil {
        fmt.Println("failed read state file name", err)
		return 
	}

    dl := new(Httpdl)

    err = json.Unmarshal(bytes, dl)
    if err != nil {
        fmt.Println("failed unmarshal json data in state file", err)
        return
    }

    dl.Do()
}

func DlTaskDo(url string, rangeSize uint, connNum int, skiptls bool) {
    dltask, err := New(url, rangeSize, connNum, skiptls)
    if err != nil {
        fmt.Println("failed create http download task", err)
        return
    }

    dltask.Do()

    for i, p := range dltask.parts {
        fmt.Println(i, p.err)
    }
}


