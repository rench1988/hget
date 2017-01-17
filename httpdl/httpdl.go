package httpdl

import (
	"crypto/tls"
	"net/http"
    "util"
    "errors"
    "os"
    "strconv"
    "path/filepath"
    "path"
    stdurl "net/url"
    "net"
    "sync"
    "fmt"
)

const (
    bufSize = 32 * 1024    //32kb buffer size for read http response body
)

var (
	tr = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client = &http.Client{Transport: tr}
)

var (
	acceptRangeHeader   = "Accept-Ranges"
	contentLengthHeader = "Content-Length"
)

var errorCheckSum error = errors.New("file content checksum failure")

type Httpdl struct {
    url        string
    fd        *os.File 
    file       string
    par        int
    len        int64
    rsize      uint
    maxconn    int
    ips        []string
    parts      []HttpdlPart
    resumable  bool
    skipTls    bool
}

type HttpdlPart struct {
	Url       string
    File      string
	RangeFrom int64
	RangeTo   int64

    err       error
    retry     int
}

func dlPart(dl *Httpdl) []HttpdlPart {
    if !dl.resumable {
        return []HttpdlPart{HttpdlPart{Url: dl.url, File: dl.file, RangeFrom: 0, RangeTo: -1}}
    }

    parts := make([]HttpdlPart, dl.par)

    for i := 0; i < dl.par; i++ {
        parts[i].RangeFrom = int64(i) * int64(dl.rsize)

        if i == dl.par - 1 {
            parts[i].RangeTo = dl.len - 1
        } else {
            parts[i].RangeTo = int64(i + 1) * int64(dl.rsize) - 1
        }

        parts[i].Url = dl.url
    }

    return parts
}

func dlInfo(dl *Httpdl) (err error) {
    req, err := http.NewRequest("GET", dl.url, nil)
    if err != nil {
        return err
    }

    resp, err := client.Do(req)
    if err != nil {
        return err
    }

    defer resp.Body.Close()

    if resp.Header.Get(acceptRangeHeader) == "" {
        dl.resumable = false
        dl.par = 1
    }

    clen := resp.Header.Get(contentLengthHeader)
    if clen == "" {
        dl.resumable = false
        dl.par = 1
        clen = "0"
    }

    len, err := strconv.ParseInt(clen, 10, 64)
    if err != nil {
        return err
    }

    pNum := len / int64(dl.rsize)
    if len % int64(dl.rsize) != 0 {
        pNum += 1
    }

    file := filepath.Base(dl.url)

    if dl.par == 0 {
        dl.par = int(pNum)
    }

    dl.len = len
    dl.file = file
    dl.parts = dlPart(dl)

    return nil
}

func dlFile(dl *Httpdl) (err error) {
    fullpath, err := filepath.Abs(filepath.Join(os.Getenv("HOME"), dataFolder, dl.file))
    if err != nil {
        return err
    }

    if err := os.MkdirAll(path.Dir(fullpath), 0770); err != nil {
		return err
	}

    temp := fullpath + ".tmp"

	fd, err := os.Create(temp)
	if err != nil {
		return err
	}

    dl.file = fullpath
    dl.fd = fd

    return nil 
}

func New(url string, rangeSize uint, connNum int, skipTls bool) (dl *Httpdl, err error) {
    parsed, err := stdurl.Parse(url)
    if err != nil {
        return nil, err 
    }

    ips, err := net.LookupIP(parsed.Host)
    if err != nil {
        return nil, err
    }

    ipstr := util.FilterIPV4(ips)

    dl = new(Httpdl)

    dl.url = url
    dl.ips = ipstr
    dl.skipTls = skipTls
    dl.resumable = true
    dl.rsize = rangeSize
    dl.maxconn = connNum

    err = dlInfo(dl)
    if err != nil {
        return nil, err
    }

    err = dlFile(dl)
    if err != nil {
        return nil, err
    }

    return dl, nil
}

func (dl *Httpdl) download(part *HttpdlPart) {
    
}

func (dl *Httpdl) Do() {
    var wg sync.WaitGroup

    consem := make(chan bool, dl.maxconn)

    for i, _ := range dl.parts {
        wg.Add(1)

        go func(dl *Httpdl, part *HttpdlPart) {
            defer wg.Done()

            consem <- true

	        defer func() { <-consem }()

            var ranges string
            if part.RangeTo != -1 {
				ranges = fmt.Sprintf("bytes=%d-%d", part.RangeFrom, part.RangeTo)
			} else {
				ranges = fmt.Sprintf("bytes=%d-", part.RangeFrom) //get all
			}

            req, err := http.NewRequest("GET", part.Url, nil)
            if err != nil {
                part.err = err
                return
            }

            if dl.par > 1 {
				req.Header.Add("Range", ranges)
			}

            resp, err := client.Do(req)
            if err != nil {
                part.err = err
				return
			}

            defer resp.Body.Close()

            var buf = make([]byte, bufSize)

            for {
                w, err := resp.Body.Read(buf)

                dl.fd.WriteAt(buf[:w], part.RangeFrom)

                part.RangeFrom += int64(w)

                if err != nil {
                    part.err = err
			        return
		        }

                if part.RangeTo != -1 && part.RangeFrom > part.RangeTo {
                    return 
                }
            }
        }(dl, &dl.parts[i])
    }

    wg.Wait()

    return
}
