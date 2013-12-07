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
    Mem uint32 `json:"mem"bson:"mem"` // MB

    ProId int `json:"pro_id"bson:"pro_id"`
}

type ProId struct {
    ProId int `json:"pro_id"bson:"pro_id"`
}

// POST /new
func (s *RsServer) newProblem(w http.ResponseWriter, r *http.Request) {
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

    problem.ProId, err = s.getProId()
    if err != nil {
        http.Error(w, "internal error", 599)
        s.logger.Println("s.getProId failed", err)
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

    b, err := json.Marshal(&ProId{problem.ProId})
    if err != nil {
        s.logger.Println("json.Marshal failed", err)
        http.Error(w, "server error", 599)
        return
    }

    w.WriteHeader(200)
    w.Write(b)
}

func (s *RsServer) getProId() (id int, err error) {

    // TODO: lock
    var idret ProId
    c := s.session.DB("oj_rs").C("pro_id")
    err = c.Find(nil).One(&idret)
    if err != nil {
        return
    }

    id = idret.ProId
    err = c.Update(bson.M{"pro_id": idret.ProId}, bson.M{"pro_id": idret.ProId+1})
    if err != nil {
        return
    }
    return
}


var proSelector = bson.M{"_id": 0}
var sortByPid = "pro_id"

// GET /list/limit/<limit>/last/<last_pid>/source/<source>
func (s *RsServer) list(w http.ResponseWriter, r *http.Request) {
    args := parseUrl(r.URL.Path[1:])

    query, err := checkProQuery(args)
    if err != nil {
        http.Error(w, "args error", 400)
        return
    }

    q := s.session.DB("oj_rs").C("problems").Find(query).Select(proSelector).Sort(sortByPid)
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
        ret["last"] = rst[limit-1].ProId
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
        query["pro_id"] = bson.M{"$gt": last}
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

    http.HandleFunc("/new", s.newProblem)
    http.HandleFunc("/list/", s.list)
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
    c := session.DB("oj_rs").C("pro_id")
    err = c.Find(bson.M{}).One(rst)
    if err != nil {
        err = c.Insert(bson.M{"pro_id": 0})
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

