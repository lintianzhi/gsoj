package main

import (
    "fmt"
    "io"
    "log"
    "os"
    "net/http"
    "strings"
    "strconv"
    "encoding/json"
    "labix.org/v2/mgo"
    "labix.org/v2/mgo/bson"
)

type Config struct {
    session *mgo.Session
    logger *log.Logger
}

type RsServer struct {
    Config
}

func loadJson(r io.Reader, v interface{}) (err error) {
    dec := json.NewDecoder(r)
    for {
        if err = dec.Decode(v); err == io.EOF {
            err = nil
            return
        } else {
            return
        }
    }
}

type Problem struct {
    Title string `json:"title"bson:"title"`
    Source string `json:"source"bson:"source"`
    Description string `json:"description"bson:"description"`
    DesIn string `json:"des_in"bson:"des_in"`
    DesOut string `json:"des_out"bson:"des_out"`

    SampleIn string `json:"sample_in"bson:"sample_in"`
    SampleOut string `json:"sample_out"bson:"sample_out"`
    Input string `json:"input"bson:"input"`
    Output string `json:"output"bson:"output"`

    Time uint8 `json:"time"bson:"time"` // second
    Memory int `json:"memory"bson:"memory"` // MB

    Submit int `json:"submit"bson:"submit"`
    Solved int `json:"solved"bson:"solved"`

    PId int `json:"pid"bson:"pid"`
}

type PId struct {
    PId int `json:"pid"bson:"pid"`
}

// POST /pnew
func (s *RsServer) pNew(w http.ResponseWriter, r *http.Request) {
    if r.Method != "POST" {
        http.Error(w, "only support POST method", 403)
        return
    }

    var problem Problem
    err := loadJson(r.Body, &problem)
    if err != nil {
        http.Error(w, "invalid argument", 400)
        return
    }

    problem.PId, err = s.getPId()
    if err != nil {
        http.Error(w, "internal error", 599)
        s.logger.Println("s.getPId failed", err)
        return
    }

    s.logger.Println(problem)
    c := s.session.DB("oj_rs").C("problems")
    err = c.Insert(&problem)
    if err != nil {
        s.logger.Println("mongo err", err)
        http.Error(w, "server error", 599)
        return
    }

    b, err := json.Marshal(&PId{problem.PId})
    if err != nil {
        s.logger.Println("json.Marshal failed", err)
        http.Error(w, "server error", 599)
        return
    }

    w.WriteHeader(200)
    w.Write(b)
}

func (s *RsServer) getPId() (id int, err error) {

    // TODO: lock
    var idret PId
    c := s.session.DB("oj_rs").C("pid")
    err = c.Find(nil).One(&idret)
    if err != nil {
        return
    }

    id = idret.PId
    err = c.Update(bson.M{"pid": idret.PId}, bson.M{"pid": idret.PId+1})
    if err != nil {
        return
    }
    return
}

// POST /pupdate/<pid>
func (s *RsServer) pUpdate(w http.ResponseWriter, r *http.Request) {

    pid, err := strconv.Atoi(r.URL.Path[9:])
    if err != nil {
        http.Error(w, "invalid args", 400)
        return
    }

    var p1 Problem
    err = loadJson(r.Body, &p1)
    if err != nil {
        http.Error(w, "invalid args", 400)
        return
    }

    p2 := make(map[string]interface{})
    if p1.Title != "" {
        p2["title"] = p1.Title
    }
    if p1.Source != "" {
        p2["source"] = p1.Source
    }
    if p1.Description != "" {
        p2["description"] = p1.Description
    }
    if p1.DesIn != "" {
        p2["des_in"] = p1.DesIn
    }
    if p1.DesOut != "" {
        p2["des_out"] = p1.DesOut
    }
    if p1.SampleIn != "" {
        p2["sample_in"] = p1.SampleIn
    }
    if p1.SampleOut != "" {
        p2["sample_out"] = p1.SampleOut
    }
    if p1.Input != "" {
        p2["input"] = p1.Input
    }
    if p1.Output != "" {
        p2["output"] = p1.Output
    }
    if p1.Memory > 0 {
        p2["memory"] = p1.Memory
    }
    if p1.Time > 0 {
        p2["time"] = p1.Time
    }

    fmt.Println(pid)
    err = s.session.DB("oj_rs").C("problems").Update(bson.M{"pid": pid}, bson.M{"$set": p2})
    if err == mgo.ErrNotFound {
        http.Error(w, "not found problem", 400)
        return
    } else if err != nil {
        http.Error(w, "internal error", 599)
        s.logger.Println("mgo update failed:", err)
        return
    }

    w.WriteHeader(200)
}

var pGetSelector = bson.M{"_id": 0}
var pListSelector = bson.M{"_id": 0, "pid": 1, "title": 1, "source": 1, "submit": 1, "solved": 1}
var sortByPid = "pid"

// get /pget/<pid>
func (s *RsServer) pGet(w http.ResponseWriter, r *http.Request) {

    pid, err := strconv.Atoi(r.URL.Path[6:])
    if err != nil {
        http.Error(w, "invalid args", 400)
        return
    }
    fmt.Println("pget", pid)

    var problem Problem
    err = s.session.DB("oj_rs").C("problems").Find(bson.M{"pid": pid}).Select(pGetSelector).One(&problem)
    if err != nil {
        fmt.Println(err)
        http.Error(w, "cannot find such problem", 430)
        return
    }

    b, err := json.Marshal(&problem)
    if err != nil {
        http.Error(w, "internal error", 599)
        s.logger.Println("json.Marshal error:", err, problem)
        return
    }

    w.WriteHeader(200)
    w.Write(b)
}

// GET /plist/limit/<limit>/last/<last_pid>/source/<source>
func (s *RsServer) pList(w http.ResponseWriter, r *http.Request) {
    args := parseUrl(r.URL.Path[1:])

    query, err := checkProQuery(args)
    if err != nil {
        http.Error(w, "args error", 400)
        return
    }

    q := s.session.DB("oj_rs").C("problems").Find(query).Select(pListSelector).Sort(sortByPid)
    var limit int
    if v, ok := args["limit"]; ok {
        limit, err = strconv.Atoi(v)
        if err != nil {
            http.Error(w, "args error", 400)
            return
        }
        q = q.Limit(limit)
    }

    var rst []*Problem
    err = q.All(&rst)
    if err != nil {
        http.Error(w, "internal error", 599)
        s.logger.Println("query error:", err)
        return
    }

    ret := make(map[string]interface{})

    ret["items"] = rst
    if limit > 0 && len(rst) == limit {
        ret["last"] = rst[limit-1].PId
    }

    b, _ := json.Marshal(&ret)
    w.WriteHeader(200)
    w.Write(b)
}

func checkProQuery(args map[string]string) (query bson.M, err error) {

    query = make(bson.M)
    if args["source"] != "" {
        query["source"] = args["source"]
    }
    if args["last"] != "" {
        var last int
        last, err = strconv.Atoi(args["last"])
        if err != nil {
            return
        }
        query["pid"] = bson.M{"$gt": last}
    }
    return
}

func parseUrl(url string) (args map[string]string) {
    args = make(map[string]string)
    list := strings.Split(url, "/")

    for i := 2; i<len(list); i+=2 {
        fmt.Println(i)
        args[list[i-1]] = list[i]
    }
    return
}

func (s *RsServer) Register() {

    http.HandleFunc("/pnew", s.pNew)
    http.HandleFunc("/pupdate/", s.pUpdate)
    http.HandleFunc("/plist/", s.pList)
    http.HandleFunc("/pget/", s.pGet)
}

func NewRsServer() (s *RsServer, err error) {
file, err := os.OpenFile("rs.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
    if err != nil {
        return
    }

    logger := log.New(file, "RS-", 1)

    session, err := mgo.Dial("localhost")
    if err != nil {
        logger.Println(err)
        return
    }

    err = initDB(session)
    if err != nil {
        logger.Println(err)
        return
    }

    return &RsServer{Config{session, logger}}, nil
}

func initDB(session *mgo.Session) (err error) {
    var rst interface{}
    c := session.DB("oj_rs").C("pid")
    err = c.Find(bson.M{}).One(rst)
    if err != nil {
        err = c.Insert(bson.M{"pid": 0})
        if err != nil {
            return
        }
    }
    return
}

func main() {
    s, err := NewRsServer()
    if err != nil {
        fmt.Println(err)
        os.Exit(0)
        panic(err)
    }
    s.Register()
    fmt.Println("RsServer run on :8080")
    panic(http.ListenAndServe(":8080", nil))
}

