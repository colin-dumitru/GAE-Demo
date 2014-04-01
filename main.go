package pc_demo

import (
	"fmt"
	"time"
	"strings"
	"net/http"
	"io/ioutil"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"

	"text/template"

    "appengine"
    "appengine/user"
    "appengine/blobstore"
    "appengine/datastore"
    "appengine/delay"
    "appengine/channel"
)

const ExpectedHash = "pSexrq_XQnqBOAXX3pv0k65Pj0A="

type TestResult struct {
    User       string
    Score      int        `datastore:",noindex"`
    TimePosted time.Time  
    Hash       string     `datastore:",noindex"`
}

var indexTmpl = template.Must(template.ParseFiles("assets/index.html"))

var scoreFunc = delay.Func("main_queue", func(c appengine.Context, blobkey string, u user.User) {
	key := appengine.BlobKey(blobkey)
	reader := blobstore.NewReader(c, key)

	content, err := ioutil.ReadAll(reader); if err != nil {
		return
	}

	decompressed := decompress(string(content))

	hasher := sha1.New()
    hasher.Write([]byte(decompressed))
    sha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))
    score := 0

    if sha == ExpectedHash {
    	score = int((1.0 / float32(len(content))) * 1000000)
    }

    result :=  TestResult {
        User: u.Email,
    	Score: score,
    	TimePosted: time.Now(),
    	Hash: sha,
    }

    time.Sleep(2 * time.Second)

    datastore.Put(c, datastore.NewIncompleteKey(c, "result", nil), &result)	

    channel.SendJSON(c, u.ID, result)
})

func decompress(input string) string {
	lines := strings.Split(input, "\n")
	decompressed := []string{}

	for _, line := range lines {
		columns := strings.Split(line, ",")

		decompressed = append(
			decompressed,
			expandLine(columns)...,
		)
	}

	return strings.Join(decompressed, "\n")
}

func expandLine(columns []string) []string {
	
	columnValues := strings.Split(columns[0], "|")

	if len(columns) == 1 {
		return columnValues
	}

	expanded := []string{}
	columns = columns[1:]

	for _, value := range columnValues {
		for _, child := range expandLine(columns) {
			expanded = append(
				expanded,
				strings.Join([]string{value, child}, ","),
			)
		}
	}

	return expanded
}

func init() {
	http.HandleFunc("/", root)
	http.HandleFunc("/upload", upload)
	http.HandleFunc("/scores", scores)
}

func auth(w http.ResponseWriter, r *http.Request) (appengine.Context, *user.User) {
	c := appengine.NewContext(r)
	u := user.Current(c)

    if u == nil {
        url, err := user.LoginURL(c, r.URL.String())
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return nil, nil
        }
        w.Header().Set("Location", url)
        w.WriteHeader(http.StatusFound)
        return nil, nil
    }

    return c, u
}

// GET /
func root(w http.ResponseWriter, r *http.Request) {
    c, u := auth(w, r)

    if u == nil {
    	return
    }

    // setup upload
    uploadURL, err := blobstore.UploadURL(c, "/upload", nil); if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
	}   

    // setup channel
    tok, err := channel.Create(c, u.ID); if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    tc := make(map[string]interface{})
	tc["Name"] = u
	tc["UploadURL"] = uploadURL
    tc["ChannelToken"] = tok
    tc["ExpectedHash"] = ExpectedHash    

	if err := indexTmpl.Execute(w, tc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// POST /upload
func upload(w http.ResponseWriter, r *http.Request) {
    c, u := auth(w, r)

    if u == nil {
    	return
    }

    blobs, _, err := blobstore.ParseUpload(r)
    if err != nil {
        return
    }

    file := blobs["file"]

    if len(file) == 0 {
        c.Errorf("no file uploaded")
        http.Redirect(w, r, "/", http.StatusFound)
        return 
    }

    scoreFunc.Call(c, string(file[0].BlobKey), *u) 

    http.Redirect(w, r, "/", http.StatusFound)
}

// GET /scores
func scores(w http.ResponseWriter, r *http.Request) {
	c, u := auth(w, r)

    if u == nil {
    	return
    }

    query := datastore.NewQuery("result").
        Filter("User =", u.Email).
        Order("TimePosted")

 	var results []TestResult
    _, err := query.GetAll(c, &results); if err != nil {
    	http.Error(w, err.Error(), http.StatusInternalServerError)
    	return;
    }

    binary, err := json.Marshal(results)

    fmt.Fprintf(w, string(binary))
}