package zv

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/urlfetch"
)

func init() {
	http.HandleFunc("/view", viewHandler)
	http.HandleFunc("/zip", zipHandler)
	http.HandleFunc("/file", fileHandler)
	http.HandleFunc("/read", readHandler)
}

func viewHandler(w http.ResponseWriter, r *http.Request) {
}

func zipHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	src, err := url.Parse(r.FormValue("url"))
	// TODO: check src.Host domain.
	if err != nil {
		serveError(c, w, err)
		return
	}

	k := datastore.NewKey(c, "Zip", src.String(), 0, nil)
	z := new(Zip)
	err = datastore.Get(c, k, z)
	if (err == datastore.ErrNoSuchEntity) || z.Expires.Before(time.Now()) {
		readHandler(w, r)
	} else if err != nil {
		serveError(c, w, err)
		return
	}

	if err := json.NewEncoder(w).Encode(z); err != nil {
		serveError(c, w, err)
		return
	}
}

func fileHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	src, err := url.Parse(r.FormValue("url"))
	// TODO: check src.Host domain.
	if err != nil {
		serveError(c, w, err)
		return
	}

	pk := datastore.NewKey(c, "Zip", src.String(), 0, nil)
	k := datastore.NewKey(c, "File", r.FormValue("file"), 0, pk)
	f := new(File)
	err = datastore.Get(c, k, f)
	if err != nil {
		serveError(c, w, err)
		return
	}

	if err := json.NewEncoder(w).Encode(f); err != nil {
		serveError(c, w, err)
		return
	}
}

// readHandler fetches a zip file, and lists its contents.
func readHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	// TODO: do this with task queues.

	src, err := url.Parse(r.FormValue("url"))
	// TODO: check src.Host domain.
	if err != nil {
		serveError(c, w, err)
		return
	}

	resp, err := urlfetch.Client(c).Get(src.String())
	if err != nil {
		serveError(c, w, err)
		return
	}

	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		serveError(c, w, err)
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(b), resp.ContentLength)
	if err != nil {
		serveError(c, w, err)
		return
	}

	entities := make([]interface{}, 0)
	keys := make([]*datastore.Key, 0)

	// TODO: use datastore transactions

	z := new(Zip)
	entities = append(entities, z)
	k := datastore.NewKey(c, "Zip", src.String(), 0, nil)
	keys = append(keys, k)

	z.Files = make([]string, 0)
	z.Fetched = time.Now()
	z.Expires = z.Fetched.Add(time.Minute * 5)

	for _, zf := range zr.File {
		if zf.Mode().IsDir() {
			continue
		}
		z.Files = append(z.Files, zf.Name)
		f, err := readFile(zf)
		if err != nil {
			serveError(c, w, err)
			return
		}
		keys = append(keys, datastore.NewKey(c, "File", f.Name, 0, k))
		entities = append(entities, f)
	}

	if err := putMulti(c, keys, entities); err != nil {
		serveError(c, w, err)
		return
	}
}

func putMulti(c appengine.Context, k []*datastore.Key, e []interface{}) (err error) {
	for len(k) > 50 {
		kk := k[:50]
		ee := e[:50]
		_, err = datastore.PutMulti(c, kk, ee)
		if err != nil {
			return err
		}
		fmt.Printf("put %d\n", len(kk))
		k = k[50:len(k)]
		e = e[50:len(e)]
	}
	if len(k) != 0 {
		_, err = datastore.PutMulti(c, k, e)
		fmt.Printf("put %d\n", len(k))
	}
	return err
}

func readFile(zf *zip.File) (f *File, err error) {
	f = new(File)
	f.Name = zf.Name
	if zf.UncompressedSize > 1e6 {
		f.TooLarge = true
		return f, nil
	}
	r, err := zf.Open()
	if err != nil {
		return nil, err
	}
	f.Content, err = ioutil.ReadAll(r)
	return
}

func serveError(c appengine.Context, w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Internal Server Error: %v", err)
	c.Errorf("%v", err)
}
