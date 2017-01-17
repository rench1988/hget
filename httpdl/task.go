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

func DlTaskResume() {
    
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


