//go:build linux

package files

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func (m *Manager) moveToTrash(username, rel string, root bool) error {
	abs, home, uid, gid, err := m.trashSource(username, rel, root)
	if err != nil {
		return err
	}
	if trashHolds(home, filepath.ToSlash(abs)) {
		return fmt.Errorf("open Trash to delete this permanently")
	}
	st, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink == 0 && abs == home {
		return fmt.Errorf("cannot delete home directory")
	}
	trashRoot := filepath.FromSlash(trashRootFor(home))
	if err := realTrashDir(trashRoot); err != nil {
		return err
	}
	id := newTrashID(time.Now())
	item := filepath.Join(trashRoot, id)
	for n := 2; n < 100; n++ {
		if _, err := os.Lstat(item); os.IsNotExist(err) {
			break
		}
		item = filepath.Join(trashRoot, fmt.Sprintf("%s-%d", id, n))
	}
	if !trashDestOK(home, filepath.ToSlash(item)) {
		return fmt.Errorf("cannot create a trash item")
	}
	from := filepath.ToSlash(abs)
	if !root {
		from = displayPath(home, from)
	}
	raw, err := json.Marshal(trashMeta{
		Name:    filepath.Base(abs),
		From:    from,
		Deleted: time.Now().UTC(),
		IsDir:   st.IsDir(),
	})
	if err != nil {
		return err
	}
	req := helperReq{Op: "trash", Path: abs, Dest: item, Home: home, Content: string(raw)}
	if root {
		_, err = runHelper(req)
		return err
	}
	_, err = m.call(uid, gid, req)
	if err != nil && permDenied(err) {
		m.ownPath(abs)
		_, err = m.call(uid, gid, req)
	}
	return err
}

func (m *Manager) ListTrash(username string, root bool) (*rpc.TrashListResp, error) {
	trashRoot, _, _, _, err := m.trashBase(username, root)
	if err != nil {
		return nil, err
	}
	items, err := readTrash(trashRoot)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []rpc.TrashItem{}
	}
	return &rpc.TrashListResp{Items: items, Days: TrashKeepDays}, nil
}

func (m *Manager) RestoreTrash(username, id string, root bool) (string, error) {
	if !validTrashID(id) {
		return "", fmt.Errorf("unknown trash item")
	}
	trashRoot, home, uid, gid, err := m.trashBase(username, root)
	if err != nil {
		return "", err
	}
	item := filepath.Join(trashRoot, id)
	meta, err := readTrashMeta(item)
	if err != nil {
		return "", fmt.Errorf("unknown trash item")
	}
	payload, err := trashPayload(item)
	if err != nil {
		return "", err
	}
	destAbs, err := m.trashRestoreDest(username, meta.From, root)
	if err != nil {
		return "", err
	}
	if protectedRoot(destAbs) || trashHolds(home, filepath.ToSlash(destAbs)) {
		return "", fmt.Errorf("cannot restore to %s", meta.From)
	}
	destAbs = uniquePath(destAbs)
	req := helperReq{Op: "rename", Path: payload, Dest: destAbs, Home: home}
	var resp *helperResp
	if root {
		got, e := runHelper(req)
		resp, err = &got, e
	} else {
		resp, err = m.call(uid, gid, req)
	}
	if err != nil {
		return "", err
	}
	if resp != nil && resp.Dest != "" {
		destAbs = resp.Dest
	}
	_ = os.RemoveAll(item)
	if root {
		return filepath.ToSlash(destAbs), nil
	}
	return displayPath(home, filepath.ToSlash(destAbs)), nil
}

func (m *Manager) DeleteTrash(username, id string, root bool) error {
	if !validTrashID(id) {
		return fmt.Errorf("unknown trash item")
	}
	trashRoot, _, _, _, err := m.trashBase(username, root)
	if err != nil {
		return err
	}
	item := filepath.Join(trashRoot, id)
	st, err := os.Lstat(item)
	if err != nil {
		return fmt.Errorf("unknown trash item")
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return fmt.Errorf("unknown trash item")
	}
	return os.RemoveAll(item)
}

func (m *Manager) EmptyTrash(username string, root bool) error {
	trashRoot, _, _, _, err := m.trashBase(username, root)
	if err != nil {
		return err
	}
	return emptyTrashDir(trashRoot)
}

func (m *Manager) trashSource(username, rel string, root bool) (abs, home string, uid, gid int, err error) {
	if root {
		abs, err = resolveRoot(rel)
		if err != nil {
			return "", "", 0, 0, err
		}
		if protectedRoot(abs) {
			return "", "", 0, 0, fmt.Errorf("cannot delete system path %s", abs)
		}
		return abs, "/", 0, 0, nil
	}
	abs, home, uid, gid, err = m.resolve(username, rel)
	if err != nil {
		return "", "", 0, 0, err
	}
	if abs == home {
		return "", "", 0, 0, fmt.Errorf("cannot delete home directory")
	}
	return abs, filepath.ToSlash(home), uid, gid, nil
}

func (m *Manager) trashBase(username string, root bool) (trashRoot, home string, uid, gid int, err error) {
	if root {
		return filepath.FromSlash(RootTrashDir), "/", 0, 0, nil
	}
	homeAbs, home, uid, gid, err := m.resolve(username, ".")
	if err != nil {
		return "", "", 0, 0, err
	}
	_ = homeAbs
	return filepath.Join(home, TrashDirName), filepath.ToSlash(home), uid, gid, nil
}

func (m *Manager) trashRestoreDest(username, from string, root bool) (string, error) {
	if root {
		return resolveRoot(from)
	}
	abs, _, _, _, err := m.resolve(username, from)
	return abs, err
}

func realTrashDir(path string) error {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return fmt.Errorf("trash folder must be a real directory")
	}
	return nil
}

func readTrash(trashRoot string) ([]rpc.TrashItem, error) {
	if err := realTrashDir(trashRoot); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(trashRoot)
	if os.IsNotExist(err) {
		return []rpc.TrashItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	var items []rpc.TrashItem
	for _, e := range entries {
		if !e.IsDir() || !validTrashID(e.Name()) {
			continue
		}
		item := filepath.Join(trashRoot, e.Name())
		meta, err := readTrashMeta(item)
		if err != nil {
			continue
		}
		info := rpc.TrashItem{
			ID:      e.Name(),
			Name:    meta.Name,
			From:    meta.From,
			IsDir:   meta.IsDir,
			Deleted: meta.Deleted.UTC().Format(time.RFC3339),
			Expires: trashExpires(meta.Deleted).Format(time.RFC3339),
		}
		if payload, err := trashPayload(item); err == nil {
			if st, err := os.Lstat(payload); err == nil && !st.IsDir() {
				info.Size = st.Size()
			}
		}
		items = append(items, info)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Deleted > items[j].Deleted
	})
	return items, nil
}

func readTrashMeta(item string) (trashMeta, error) {
	b, err := os.ReadFile(filepath.Join(item, "meta.json"))
	if err != nil {
		return trashMeta{}, err
	}
	var meta trashMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return trashMeta{}, err
	}
	if meta.Name == "" || meta.From == "" || meta.Deleted.IsZero() {
		return trashMeta{}, fmt.Errorf("incomplete trash item")
	}
	return meta, nil
}

func trashPayload(item string) (string, error) {
	dir := filepath.Join(item, "payload")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("trash item is incomplete")
	}
	if len(entries) != 1 {
		return "", fmt.Errorf("trash item is incomplete")
	}
	return filepath.Join(dir, entries[0].Name()), nil
}

func emptyTrashDir(trashRoot string) error {
	if err := realTrashDir(trashRoot); err != nil {
		return err
	}
	entries, err := os.ReadDir(trashRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var first error
	for _, e := range entries {
		if !validTrashID(e.Name()) {
			continue
		}
		item := filepath.Join(trashRoot, e.Name())
		st, err := os.Lstat(item)
		if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			continue
		}
		if err := os.RemoveAll(item); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func purgeTrashTree(trashRoot string, now time.Time) int {
	if err := realTrashDir(trashRoot); err != nil {
		return 0
	}
	entries, err := os.ReadDir(trashRoot)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() || !validTrashID(e.Name()) {
			continue
		}
		item := filepath.Join(trashRoot, e.Name())
		st, err := os.Lstat(item)
		if err != nil || st.Mode()&os.ModeSymlink != 0 {
			continue
		}
		meta, err := readTrashMeta(item)
		deleted := meta.Deleted
		if err != nil {
			info, e2 := e.Info()
			if e2 != nil {
				continue
			}
			deleted = info.ModTime()
		}
		if !trashExpired(deleted, now) {
			continue
		}
		if os.RemoveAll(item) == nil {
			n++
		}
	}
	return n
}

// PurgeTrash removes trash items older than 15 days from every home and the root file manager.
func PurgeTrash(homeRoot string) int {
	n := purgeTrashTree(filepath.FromSlash(RootTrashDir), time.Now())
	if homeRoot == "" {
		return n
	}
	homes, err := os.ReadDir(homeRoot)
	if err != nil {
		return n
	}
	for _, home := range homes {
		if !home.IsDir() {
			continue
		}
		n += purgeTrashTree(filepath.Join(homeRoot, home.Name(), TrashDirName), time.Now())
	}
	return n
}

// WatchTrash purges expired files at startup and every six hours.
func WatchTrash(homeRoot string) {
	run := func() {
		if n := PurgeTrash(homeRoot); n > 0 {
			log.Printf("trash: removed %d item(s) older than %d days", n, TrashKeepDays)
		}
	}
	run()
	tick := time.NewTicker(6 * time.Hour)
	defer tick.Stop()
	for range tick.C {
		run()
	}
}

func movePath(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	if err := copyPath(src, dest); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func helperTrash(req helperReq) (helperResp, error) {
	if !trashDestOK(req.Home, filepath.ToSlash(req.Dest)) {
		return helperResp{}, fmt.Errorf("path is outside trash")
	}
	base := filepath.Base(req.Path)
	if base == "" || base == "." || base == ".." || base == TrashDirName {
		return helperResp{}, fmt.Errorf("cannot delete %s", base)
	}
	if err := os.MkdirAll(req.Dest, 0700); err != nil {
		return helperResp{}, err
	}
	if err := os.WriteFile(filepath.Join(req.Dest, "meta.json"), []byte(req.Content), 0600); err != nil {
		_ = os.RemoveAll(req.Dest)
		return helperResp{}, err
	}
	payload := filepath.Join(req.Dest, "payload")
	if err := os.MkdirAll(payload, 0700); err != nil {
		_ = os.RemoveAll(req.Dest)
		return helperResp{}, err
	}
	if err := movePath(req.Path, filepath.Join(payload, base)); err != nil {
		_ = os.RemoveAll(req.Dest)
		return helperResp{}, err
	}
	return helperResp{OK: true}, nil
}
