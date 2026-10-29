package lockfile

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Doc is the document a window has open. It registers the file in this
// process, so that opening it again raises the window, and in a per-user
// directory (one small file per open document: pid, path and how to reach
// this process), so that other OfficeLite processes can do the same; and
// it holds the document's owner file, if it writes one. A nil Doc does
// nothing.
type Doc struct {
	raise func()

	mu    sync.Mutex
	path  string // "" when no file is open
	key   string
	entry string // the registry file
	own   *owner
}

// NewDoc makes the tracker of a window; raise brings the window to the
// front (it is called from any goroutine).
func NewDoc(raise func()) *Doc { return &Doc{raise: raise} }

// Set makes path the window's file ("" for none), dropping the previous
// one's registration and owner file. With lock, an owner file is taken
// for it (on a network share); without, one held is let go (read-only).
func (d *Doc) Set(path string, lock bool) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if path != "" {
		if p, err := filepath.Abs(path); err == nil {
			path = p
		}
	}
	if path != d.path {
		d.unregister()
		d.own.release()
		d.own = nil
		d.path = path
		if path != "" {
			d.register()
		}
	}
	switch {
	case lock && d.own == nil && path != "":
		d.own = acquireOwner(path)
	case !lock && d.own != nil:
		d.own.release()
		d.own = nil
	}
}

// Path is the file Set last registered.
func (d *Doc) Path() string {
	if d == nil {
		return ""
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.path
}

// Close lets the file go, when the window closes.
func (d *Doc) Close() { d.Set("", false) }

func (d *Doc) register() {
	d.key = key(d.path)
	reg.Lock()
	if reg.docs == nil {
		reg.docs = map[string]*Doc{}
	}
	if _, ok := reg.docs[d.key]; !ok {
		reg.docs[d.key] = d
	}
	reg.Unlock()
	port, token := listen()
	dir, err := registryDir()
	if err != nil || os.MkdirAll(dir, 0o700) != nil {
		return
	}
	b, _ := json.Marshal(entry{PID: os.Getpid(), Path: d.path, Port: port, Token: token})
	name := filepath.Join(dir, hashKey(d.key)+"-"+strconv.Itoa(os.Getpid())+"-"+strconv.Itoa(int(docSeq.Add(1))))
	if os.WriteFile(name, b, 0o600) == nil {
		d.entry = name
	}
}

func (d *Doc) unregister() {
	if d.path == "" {
		return
	}
	reg.Lock()
	if reg.docs[d.key] == d {
		delete(reg.docs, d.key)
	}
	reg.Unlock()
	if d.entry != "" {
		os.Remove(d.entry)
	}
	d.entry, d.key = "", ""
}

// reg is this process's open documents, and its hand-off listener.
var reg struct {
	sync.Mutex
	docs   map[string]*Doc // by key
	ln     net.Listener
	port   int
	token  string
	listed bool
}

// docSeq numbers this process's registry files.
var docSeq atomic.Int32

// entry is a registry file's content.
type entry struct {
	PID   int    `json:"pid"`
	Path  string `json:"path"`
	Port  int    `json:"port,omitempty"`  // localhost port taking hand-offs
	Token string `json:"token,omitempty"` // proves the sender read the registry
}

// registryDir is where the per-user registry lives.
var registryDir = func() (string, error) {
	dir, err := os.UserCacheDir()
	return filepath.Join(dir, "officelite", "open"), err
}

// key identifies a file across windows: its absolute path, without case
// on Windows.
func key(path string) string {
	if p, err := filepath.Abs(path); err == nil {
		path = p
	}
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

func hashKey(k string) string {
	h := sha256.Sum256([]byte(k))
	return hex.EncodeToString(h[:12])
}

// Activate brings to the front the window that has path open, in this
// process or in another OfficeLite process, and reports whether there
// was one. Callers open the file only when there wasn't.
func Activate(path string) bool {
	k := key(path)
	reg.Lock()
	d := reg.docs[k]
	reg.Unlock()
	if d != nil {
		d.raise()
		return true
	}
	for _, e := range others(k) {
		if handOff(e, path) {
			return true
		}
	}
	return false
}

// PIDAlive reports whether process pid is running; 0 and negative pids
// never are. AutoRecover uses it to tell a crashed OfficeLite's files.
func PIDAlive(pid int) bool { return pid > 0 && pidAlive(pid) }

// openElsewhere reports whether another live OfficeLite process has the
// file open.
func openElsewhere(path string) bool { return len(others(key(path))) > 0 }

// others reads the registry entries of other running processes for a
// file, deleting those whose process is gone.
func others(k string) []entry {
	dir, err := registryDir()
	if err != nil {
		return nil
	}
	names, _ := filepath.Glob(filepath.Join(dir, hashKey(k)+"-*"))
	var out []entry
	for _, name := range names {
		var e entry
		b, err := os.ReadFile(name)
		if err != nil || json.Unmarshal(b, &e) != nil || key(e.Path) != k {
			continue
		}
		if e.PID == os.Getpid() {
			continue
		}
		if !pidAlive(e.PID) {
			os.Remove(name)
			continue
		}
		out = append(out, e)
	}
	return out
}

// listen starts the hand-off listener on first use and returns its port
// and token (0 and "" if it can't listen).
func listen() (int, string) {
	reg.Lock()
	defer reg.Unlock()
	if reg.listed {
		return reg.port, reg.token
	}
	reg.listed = true
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, ""
	}
	var t [16]byte
	rand.Read(t[:])
	reg.ln, reg.port, reg.token = ln, ln.Addr().(*net.TCPAddr).Port, hex.EncodeToString(t[:])
	go serve(ln, reg.token)
	return reg.port, reg.token
}

// The hand-off protocol: the sender writes the token and the path, a line
// each; the receiver raises the file's window and answers "ok", or "no"
// when it hasn't got the file open.
const handOffTimeout = 2 * time.Second

func serve(ln net.Listener, token string) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			c.SetDeadline(time.Now().Add(handOffTimeout))
			r := bufio.NewReader(&limitedConn{c, 8 << 10})
			tok, err1 := r.ReadString('\n')
			path, err2 := r.ReadString('\n')
			if err1 != nil || err2 != nil || strings.TrimSuffix(tok, "\n") != token {
				return
			}
			k := key(strings.TrimSuffix(path, "\n"))
			reg.Lock()
			d := reg.docs[k]
			reg.Unlock()
			if d == nil {
				c.Write([]byte("no\n"))
				return
			}
			d.raise()
			c.Write([]byte("ok\n"))
		}()
	}
}

// limitedConn reads at most n bytes of a connection.
type limitedConn struct {
	net.Conn
	n int
}

func (l *limitedConn) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, net.ErrClosed
	}
	if len(p) > l.n {
		p = p[:l.n]
	}
	n, err := l.Conn.Read(p)
	l.n -= n
	return n, err
}

// handOff asks the process of e to raise its window for path.
func handOff(e entry, path string) bool {
	if e.Port == 0 {
		return false
	}
	allowForeground(e.PID)
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(e.Port), handOffTimeout)
	if err != nil {
		return false
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(handOffTimeout))
	if _, err := c.Write([]byte(e.Token + "\n" + path + "\n")); err != nil {
		return false
	}
	reply, _ := bufio.NewReader(c).ReadString('\n')
	return reply == "ok\n"
}
