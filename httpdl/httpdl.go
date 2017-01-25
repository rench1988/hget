package httpdl

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	stdurl "net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"
	"util"
)

const (
	bufSize = 4096 //32kb buffer size for read http response body

	retryNum = 1000 //retry times for every http request

	defaultRedirectLimit = 30

	reqTimeout = 300 * time.Second
)

var (
	tr = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	client = &http.Client{Transport: tr, CheckRedirect: addDlheader}
)

var (
	acceptRangeHeader   = "Accept-Ranges"
	contentLengthHeader = "Content-Length"
)

var errorCheckSum error = errors.New("file content checksum failure")
var errorUnexpectedClose error = errors.New("server closed the request connection unexpectedly")
var errorBadStatus error = errors.New("bad response status code")

type Httpdl struct {
	Url       string `json:"url"`
	fd        *os.File
	File      string `json:"filename"`
	Par       int    `json:"partnum"`
	Len       int64  `json:"filelen"`
	Rsize     uint   `json:"range-size"`
	Maxconn   int    `json:"maxconn"`
	connsem   chan bool
	errs      chan error
	ips       []string
	Parts     []HttpdlPart `json:"parts"`
	resumable bool
	skipTls   bool
}

type HttpdlPart struct {
	RangeFrom int64 `json:"from"`
	RangeTo   int64 `json:"to"`

	err   error
	retry int
}

func addDlheader(req *http.Request, via []*http.Request) error {
	if len(via) > defaultRedirectLimit {
		return fmt.Errorf("%d consecutive requests(redirects)", len(via))
	}
	if len(via) == 0 {
		// No redirects
		return nil
	}
	if v, ok := via[0].Header["Range"]; ok {
		req.Header["Range"] = v
	}

	return nil
}

func (dl *Httpdl) dlRepart() {
	nparts := make([]HttpdlPart, 0)

	for i := 0; i < len(dl.Parts); i++ {
		if dl.Parts[i].RangeFrom <= dl.Parts[i].RangeTo {
			nparts = append(nparts, dl.Parts[i])
		}
	}

	dl.Parts = nparts
}

func dlPart(dl *Httpdl) []HttpdlPart {
	if !dl.resumable {
		return []HttpdlPart{HttpdlPart{RangeFrom: 0, RangeTo: -1}}
	}

	parts := make([]HttpdlPart, dl.Par)

	for i := 0; i < dl.Par; i++ {
		parts[i].RangeFrom = int64(i) * int64(dl.Rsize)

		if i == dl.Par-1 {
			parts[i].RangeTo = dl.Len - 1
		} else {
			parts[i].RangeTo = int64(i+1)*int64(dl.Rsize) - 1
		}
	}

	return parts
}

/*
func dlPart(dl *Httpdl) []HttpdlPart {
	var (
		maxRoutine int = 128
		parts      []HttpdlPart
	)

	if !dl.resumable {
		return []HttpdlPart{HttpdlPart{RangeFrom: 0, RangeTo: -1}}
	}

	if dl.Len > 0 {
		persize := dl.Len / int64(maxRoutine)

		parts = make([]HttpdlPart, maxRoutine)

		var i int
		for i = 0; i < maxRoutine-1; i++ {
			parts[i].RangeFrom = int64(persize) * int64(i)
			parts[i].RangeTo = int64(persize)*int64(i+1) - 1
		}

		parts[i].RangeFrom = int64(i) * int64(persize)
		parts[i].RangeTo = int64(dl.Len - 1)
	}

	dl.Par = len(parts)

	return parts
}
*/

func dlInfo(dl *Httpdl) (err error) {
	req, err := http.NewRequest("GET", dl.Url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return errorBadStatus
	}

	if resp.Header.Get(acceptRangeHeader) == "" {
		dl.resumable = false
		dl.Par = 1
	}

	clen := resp.Header.Get(contentLengthHeader)
	if clen == "" {
		dl.resumable = false
		dl.Par = 1
		clen = "0"
	}

	len, err := strconv.ParseInt(clen, 10, 64)
	if err != nil {
		return err
	}

	pNum := len / int64(dl.Rsize)
	if len%int64(dl.Rsize) != 0 {
		pNum += 1
	}

	file := filepath.Base(dl.Url)

	if dl.Par == 0 {
		dl.Par = int(pNum)
	}

	dl.Len = len
	dl.File = file
	dl.Parts = dlPart(dl)

	return nil
}

func dlFile(dl *Httpdl) (err error) {
	fullpath, err := filepath.Abs(filepath.Join(os.Getenv("HOME"), dataFolder, filepath.Base(dl.File)))
	if err != nil {
		return err
	}

	if err := os.MkdirAll(path.Dir(fullpath), 0770); err != nil {
		return err
	}

	temp := fullpath + ".tmp"

	var fd *os.File

	if _, err = os.Stat(temp); os.IsNotExist(err) {
		fd, err = os.Create(temp)
		if err != nil {
			return err
		}
	} else {
		fd, err = os.OpenFile(temp, os.O_RDWR, 0666)
		if err != nil {
			return err
		}
	}

	dl.File = fullpath
	dl.fd = fd

	return nil
}

func New(url string, rangeSize uint, connNum int, skipTls bool) (dl *Httpdl, err error) {
	parsed, err := stdurl.Parse(url)
	if err != nil {
		return nil, err
	}

	host, _, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return nil, err
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}

	ipstr := util.FilterIPV4(ips)

	dl = new(Httpdl)

	dl.Url = url
	dl.ips = ipstr
	dl.skipTls = skipTls
	dl.resumable = true
	dl.Rsize = rangeSize
	dl.Maxconn = connNum

	dl.connsem = make(chan bool, dl.Maxconn)

	err = dlInfo(dl)
	if err != nil {
		return nil, err
	}

	err = dlFile(dl)
	if err != nil {
		return nil, err
	}

	dl.errs = make(chan error, len(dl.Parts))

	return dl, nil
}

func (dl *Httpdl) dlRecord() (err error) {
	b, err := json.Marshal(dl)
	if err != nil {
		return err
	}

	stateFile, err := filepath.Abs(filepath.Join(os.Getenv("HOME"), dataFolder, filepath.Base(dl.Url)+".status"))
	if err != nil {
		return err
	}

	return ioutil.WriteFile(stateFile, b, 0644)
}

func (dl *Httpdl) dlRename() error {
	defer dl.fd.Close()

	return os.Rename(dl.fd.Name(), dl.File)
}

func (dl *Httpdl) dlRange(part *HttpdlPart, request *http.Request) (err error) {
	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return errorBadStatus
	}

	var w int
	var buf = make([]byte, bufSize)

	for {

		w, err = resp.Body.Read(buf)

		readSize := int64(len(buf[:w]))

		if part.RangeTo != -1 {
			needSize := part.RangeTo - part.RangeFrom + 1

			if readSize > needSize {
				w = int(needSize)
				readSize = needSize
				err = io.EOF
			}
		}

		if _, wr := dl.fd.WriteAt(buf[:w], part.RangeFrom); wr != nil {
			return wr
		}

		part.RangeFrom += readSize

		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

	}

	return nil
}

func (dl *Httpdl) download(partIndex int) {
	dl.connsem <- true

	part := &dl.Parts[partIndex]

	var (
		err error
	)

	defer func() { <-dl.connsem }()
	defer func() {
		part.err = err
		dl.errs <- part.err
	}()

	req, err := http.NewRequest("GET", dl.Url, nil)
	if err != nil {
		return
	}

	for i := 0; i < retryNum; i++ {

		if part.RangeTo != -1 {
			req.Header.Set(
				"Range",
				"bytes="+strconv.FormatInt(part.RangeFrom, 10)+"-"+strconv.FormatInt(part.RangeTo, 10),
			)
		}

		err = dl.dlRange(part, req)
		if err == nil {
			break
		}

		fmt.Println("Retry======================= ", part.retry, err)
		part.retry++
	}

	return
}

func (dl *Httpdl) Do() error {

	for i, _ := range dl.Parts {
		go dl.download(i)
	}

	st := time.Now()

	for i := 0; i < len(dl.Parts); i++ {
		err := <-dl.errs

		if dl.resumable && err == nil {
			dl.dlRecord()
		}

		fmt.Println("Left ", i, err)
	}

	et := time.Now()

	fmt.Printf("The download took %v to run.\n", et.Sub(st))

	for i := 0; i < len(dl.Parts); i++ {
		if dl.Parts[i].err != nil {
			return dl.Parts[i].err
		}
	}

	return dl.dlRename()
}
