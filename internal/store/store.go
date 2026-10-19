// Package store persists atomic encrypted configuration snapshots in SQLite.
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/uniproxy/uniproxy/internal/model"
	_ "modernc.org/sqlite"
)

type Store struct {
	mu    sync.RWMutex
	db    *sql.DB
	aead  cipher.AEAD
	state model.State
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "uniproxy.db")
	keyPath := filepath.Join(dir, "master.key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		if info, e := os.Stat(path); e == nil && info.Size() > 0 {
			return nil, errors.New("数据库存在但 master.key 缺失；请恢复原密钥，不能创建新密钥")
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		f, e := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return nil, e
		}
		_, err = f.Write(key)
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("master.key 格式无效")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.ToSlash(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(e error) (*Store, error) { db.Close(); return nil, e }
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA synchronous=FULL", "CREATE TABLE IF NOT EXISTS state (id INTEGER PRIMARY KEY CHECK(id=1), schema_version INTEGER NOT NULL, payload BLOB NOT NULL)", "CREATE TABLE IF NOT EXISTS activities (id INTEGER PRIMARY KEY AUTOINCREMENT, level TEXT NOT NULL, message TEXT NOT NULL, created_at TEXT NOT NULL)"} {
		if _, err = db.Exec(q); err != nil {
			return fail(err)
		}
	}
	_ = os.Chmod(path, 0600)
	s := &Store{db: db, aead: aead, state: model.DefaultState()}
	var payload []byte
	var version int
	err = db.QueryRow("SELECT schema_version,payload FROM state WHERE id=1").Scan(&version, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		if err = s.Save(s.state); err != nil {
			return fail(err)
		}
		return s, nil
	}
	if err != nil {
		return fail(err)
	}
	if version != 1 {
		return fail(fmt.Errorf("不支持的数据库版本 %d", version))
	}
	if len(payload) < aead.NonceSize() {
		return fail(errors.New("配置数据损坏"))
	}
	plain, err := aead.Open(nil, payload[:aead.NonceSize()], payload[aead.NonceSize():], []byte("uniproxy-state-v1"))
	if err != nil {
		return fail(errors.New("配置解密失败，密钥不匹配或数据损坏"))
	}
	if err = json.Unmarshal(plain, &s.state); err != nil {
		return fail(errors.New("配置数据格式无效"))
	}
	Normalize(&s.state)
	return s, nil
}
func Clone(state model.State) model.State {
	b, _ := json.Marshal(state)
	var out model.State
	_ = json.Unmarshal(b, &out)
	Normalize(&out)
	return out
}
func (s *Store) Snapshot() model.State { s.mu.RLock(); defer s.mu.RUnlock(); return Clone(s.state) }
func (s *Store) Save(state model.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	Normalize(&state)
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	payload := s.aead.Seal(nonce, nonce, b, []byte("uniproxy-state-v1"))
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec("INSERT INTO state(id,schema_version,payload) VALUES(1,1,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", payload)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.state = Clone(state)
	return nil
}
func (s *Store) Activity(level, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, _ = s.db.Exec("INSERT INTO activities(level,message,created_at) VALUES(?,?,?)", level, message, model.Now())
	_, _ = s.db.Exec("DELETE FROM activities WHERE id NOT IN (SELECT id FROM activities ORDER BY id DESC LIMIT 200)")
}
func (s *Store) Activities() []model.Activity {
	out := []model.Activity{}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT id,level,message,created_at FROM activities ORDER BY id DESC LIMIT 50")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var a model.Activity
		if rows.Scan(&a.ID, &a.Level, &a.Message, &a.CreatedAt) == nil {
			out = append(out, a)
		}
	}
	return out
}
func (s *Store) Close() error { return s.db.Close() }
func Normalize(s *model.State) {
	if s.Nodes == nil {
		s.Nodes = []model.Node{}
	}
	if s.Groups == nil {
		s.Groups = []model.Group{}
	}
	if s.Pools == nil {
		s.Pools = []model.Pool{}
	}
	if s.Listeners == nil {
		s.Listeners = []model.Listener{}
	}
	if s.Subscriptions == nil {
		s.Subscriptions = []model.Subscription{}
	}
	for i := range s.Nodes {
		n := &s.Nodes[i]
		if n.GroupIDs == nil {
			n.GroupIDs = []string{}
		}
		if n.ManualGroupIDs == nil {
			n.ManualGroupIDs = []string{}
		}
		if n.SourceIDs == nil {
			n.SourceIDs = []string{}
		}
		if n.SourceGroups == nil {
			n.SourceGroups = map[string][]string{}
		}
	}
	for i := range s.Pools {
		p := &s.Pools[i]
		if p.NodeIDs == nil {
			p.NodeIDs = []string{}
		}
		if p.GroupIDs == nil {
			p.GroupIDs = []string{}
		}
		if p.ExcludeNodeIDs == nil {
			p.ExcludeNodeIDs = []string{}
		}
		if p.ExcludeGroupIDs == nil {
			p.ExcludeGroupIDs = []string{}
		}
	}
	for i := range s.Listeners {
		if s.Listeners[i].Users == nil {
			s.Listeners[i].Users = []model.Credential{}
		}
	}
	for i := range s.Subscriptions {
		if s.Subscriptions[i].GroupIDs == nil {
			s.Subscriptions[i].GroupIDs = []string{}
		}
	}
}
