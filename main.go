package pc_demo

import (
	"fmt"
	"time"
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
)

const ExpectedHash = "1111"

type TestResult struct {
    Score     int
    TimePosted time.Time
    Hash string
}

var indexTmpl = template.Must(template.ParseFiles("assets/index.html"))

var scoreFunc = delay.Func("main_queue", func(c appengine.Context, blobkey string) {
	key := appengine.BlobKey(blobkey)
	reader := blobstore.NewReader(c, key)

	content, err := ioutil.ReadAll(reader); if err != nil {
		return
	}

	hasher := sha1.New()
    hasher.Write(content)
    sha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

    result :=  TestResult {
    	Score: len(content),
    	TimePosted: time.Now(),
    	Hash: sha,
    }

    datastore.Put(c, datastore.NewIncompleteKey(c, "result", nil), &result)	
})

func init() {
	http.HandleFunc("/", root)
	http.HandleFunc("/upload", upload)
	http.HandleFunc("/serve/", serve)
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

func root(w http.ResponseWriter, r *http.Request) {
    c, u := auth(w, r)

    if u == nil {
    	return
    }

    uploadURL, err := blobstore.UploadURL(c, "/upload", nil)
	if err != nil {
        return
	}   

    tc := make(map[string]interface{})
	tc["Name"] = u
	tc["UploadURL"] = uploadURL

	if err := indexTmpl.Execute(w, tc);

	err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func serve(w http.ResponseWriter, r *http.Request) {
    blobstore.Send(w, appengine.BlobKey(r.FormValue("blobKey")))
}

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

    scoreFunc.Call(c, string(file[0].BlobKey))

    http.Redirect(w, r, "/", http.StatusFound)
}

func scores(w http.ResponseWriter, r *http.Request) {
	c, u := auth(w, r)

    if u == nil {
    	return
    }

    query := datastore.NewQuery("result")

 	var results []TestResult
    _, err := query.GetAll(c, &results); if err != nil {
    	http.Error(w, err.Error(), http.StatusInternalServerError)
    	return;
    }

    binary, err := json.Marshal(results)

    fmt.Fprintf(w, string(binary))
}