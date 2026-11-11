package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"tock/internal/service"
	"tock/internal/snapshot"
	"tock/internal/store"
)

// restoreStop and restoreStart stop and start what holds tock.db open:
// trackd, and on macOS the menu bar (which runs tock web). Tests swap them.
var (
	restoreStop  = stopForRestore
	restoreStart = startAfterRestore
)

var restoreLabels = []string{"com.tock.trackd", "com.tock.bar"}

// servicesUse reports whether trackd and the menu bar use env's data dir.
// They run with the default one; a restore into another (TOCK_DATA_DIR)
// must leave them alone.
func servicesUse(env Env) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return filepath.Clean(env.DataDir) == filepath.Join(home, ".local", "share", "tock")
}

func stopForRestore(env Env) error {
	if !servicesUse(env) {
		return nil
	}
	if env.Linux {
		_, err := setupRun("systemctl", "--user", "stop", "trackd")
		return err
	}
	for _, l := range restoreLabels {
		if !loaded(l) {
			continue
		}
		if _, err := setupRun("launchctl", "bootout", guiTarget(l)); err != nil {
			return fmt.Errorf("restore: stop %s: %w", l, err)
		}
		for i := 0; loaded(l); i++ {
			if i == 20 {
				return fmt.Errorf("restore: %s still running after 10 s", l)
			}
			env.Sleep(500 * time.Millisecond)
		}
	}
	return nil
}

func startAfterRestore(env Env) error {
	if !servicesUse(env) {
		return nil
	}
	if env.Linux {
		_, err := setupRun("systemctl", "--user", "start", "trackd")
		return err
	}
	for _, l := range restoreLabels {
		p := service.PlistPath(l)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if _, err := setupRun("launchctl", "bootstrap", "gui/"+strconv.Itoa(os.Getuid()), p); err != nil {
			return fmt.Errorf("restore: start %s: %w", l, err)
		}
	}
	return nil
}

// restorable is a snapshot or a manual archive.
type restorable struct {
	path   string
	kind   string // daily, pre, manual
	atMS   int64
	schema int // 0 = unknown until extracted (manual)
	size   int64
}

func restorables(env Env) ([]restorable, error) {
	var out []restorable
	list, err := snapshot.List(store.AutoDir(env.dbPath()))
	if err != nil {
		return nil, err
	}
	for _, i := range list {
		out = append(out, restorable{i.Path, string(i.Kind), i.AtMS, i.Schema, i.Size})
	}
	manual, _ := filepath.Glob(filepath.Join(env.DataDir, "backups", "tock-*.tar.gz"))
	for _, m := range manual {
		if fi, err := os.Stat(m); err == nil {
			out = append(out, restorable{m, "manual", fi.ModTime().UnixMilli(), 0, fi.Size()})
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].atMS > out[b].atMS })
	return out, nil
}

func cmdRestore(env Env, args []string) error {
	fs := newFlags("restore")
	yes := fs.Bool("yes", false, "restore without asking")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) > 1 { // flags after the snapshot: tock restore 2 --yes
		if err := parseFlags(fs, rest[1:]); err != nil {
			return err
		}
		rest = append([]string{rest[0]}, fs.Args()...)
	}
	all, err := restorables(env)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		if len(all) == 0 {
			fmt.Fprintln(env.Stdout, "no snapshots yet; trackd takes one a day, and tock backup writes one now")
			return nil
		}
		for i, r := range all {
			schema := "?"
			if r.schema > 0 {
				schema = "v" + strconv.Itoa(r.schema)
			}
			fmt.Fprintf(env.Stdout, "%d  %s  %-6s  %-4s  %s\n", i+1,
				time.UnixMilli(r.atMS).In(env.Now().Location()).Format("Mon 2 Jan 15:04"), r.kind, schema, sizeMB(r.size))
		}
		return nil
	}
	if len(rest) > 1 {
		return usagef("restore takes one snapshot number or path")
	}
	pick, err := pickRestorable(all, rest[0])
	if err != nil {
		return err
	}
	if pick.schema > store.LatestVersion() {
		return fmt.Errorf("restore: the snapshot is from a newer tock (schema v%d, this one v%d); update tock first", pick.schema, store.LatestVersion())
	}
	if !*yes && !env.Interactive {
		return usagef("restore: confirm with --yes")
	}
	tmp := filepath.Join(env.DataDir, ".restore-tmp.db")
	_ = os.Remove(tmp)
	defer os.Remove(tmp)
	if pick.kind == "manual" {
		err = extractManual(pick.path, tmp)
	} else {
		err = snapshot.Extract(pick.path, tmp)
	}
	if err != nil {
		return err
	}
	if v, err := schemaOf(tmp); err != nil {
		return err
	} else if v > store.LatestVersion() {
		return fmt.Errorf("restore: the snapshot is from a newer tock (schema v%d, this one v%d); update tock first", v, store.LatestVersion())
	}
	when := time.UnixMilli(pick.atMS).In(env.Now().Location()).Format("Mon 2 Jan 15:04")
	keep := env.dbPath() + ".before-restore-" + env.Now().Format("20060102-150405")
	lost, err := invoicesLost(env.dbPath(), tmp)
	if err != nil {
		return err
	}
	if len(lost) > 0 {
		fmt.Fprintf(env.Stdout, "The snapshot from %s lacks %d issued invoice(s): %s.\n", when, len(lost), strings.Join(lost, ", "))
		fmt.Fprintf(env.Stdout, "Their files stay in %s, and tock invoice recover brings them back afterwards.\n", store.InvoiceDir(env.dbPath()))
		if !*yes {
			return usagef("restore: invoices would leave the database; confirm with --yes")
		}
	}
	if !*yes {
		fmt.Fprintf(env.Stdout, "This replaces tock.db with the snapshot from %s. Activity recorded after that is lost (the current database is kept as %s). Restore? [y/N] ", when, keep)
		line, _ := bufio.NewReader(env.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			fmt.Fprintln(env.Stdout, "nothing changed")
			return nil
		}
	}
	if err := restoreStop(env); err != nil {
		return err
	}
	// With trackd and the menu bar stopped, nothing else may have the
	// database open: a tock web or tock tui started by hand would go on
	// reading or writing the old file after the swap.
	// The menu bar's tock web can outlive its parent by a moment.
	release, err := store.Exclusive(env.dbPath())
	for i := 0; err != nil && i < 6; i++ {
		env.Sleep(500 * time.Millisecond)
		release, err = store.Exclusive(env.dbPath())
	}
	if err != nil {
		if startErr := restoreStart(env); startErr != nil {
			fmt.Fprintln(env.Stderr, startErr)
		}
		return fmt.Errorf("restore: %w; nothing changed", err)
	}
	swapErr := swapDB(env.dbPath(), tmp, keep)
	release()
	startErr := restoreStart(env)
	if swapErr != nil {
		return swapErr
	}
	fmt.Fprintf(env.Stdout, "restored the snapshot from %s; the old database is %s\n", when, keep)
	if pick.kind == "manual" {
		fmt.Fprintf(env.Stdout, "rules.toml and config.toml are in %s; restore them by hand if you want them.\n", pick.path)
	}
	return startErr
}

// invoicesLost lists the invoice numbers in the database at db that the
// snapshot at snap does not have.
func invoicesLost(db, snap string) ([]string, error) {
	if _, err := os.Stat(db); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	cur, err := store.InvoiceNumbersIn(db)
	if err != nil {
		return nil, fmt.Errorf("restore: %w", err)
	}
	old, err := store.InvoiceNumbersIn(snap)
	if err != nil {
		return nil, fmt.Errorf("restore: %w", err)
	}
	have := map[string]bool{}
	for _, n := range old {
		have[n] = true
	}
	var lost []string
	for _, n := range cur {
		if !have[n] {
			lost = append(lost, n)
		}
	}
	return lost, nil
}

func pickRestorable(all []restorable, arg string) (restorable, error) {
	if n, err := strconv.Atoi(arg); err == nil {
		if n < 1 || n > len(all) {
			return restorable{}, usagef("restore: no snapshot %d (tock restore lists them)", n)
		}
		return all[n-1], nil
	}
	for _, r := range all {
		if r.path == arg || filepath.Base(r.path) == arg {
			return r, nil
		}
	}
	return restorable{}, usagef("restore: no snapshot %q (tock restore lists them)", arg)
}

// swapDB moves db and its -wal/-shm to keep(+suffix) and tmp to db. If
// moving tmp in fails, the originals go back.
func swapDB(db, tmp, keep string) error {
	var moved [][2]string
	for _, sfx := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(db + sfx); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := os.Rename(db+sfx, keep+sfx); err != nil {
			undo(moved)
			return fmt.Errorf("restore: %w", err)
		}
		moved = append(moved, [2]string{db + sfx, keep + sfx})
	}
	if err := os.Rename(tmp, db); err != nil {
		undo(moved)
		return fmt.Errorf("restore: %w", err)
	}
	return nil
}

func undo(moved [][2]string) {
	for _, m := range moved {
		_ = os.Rename(m[1], m[0])
	}
}

func schemaOf(path string) (int, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("restore: %w", err)
	}
	defer db.Close()
	var v sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&v); err != nil {
		return 0, fmt.Errorf("restore: read schema: %w", err)
	}
	return int(v.Int64), nil
}

// extractManual copies the tock.db entry of a tock backup archive to dst
// and checks it.
func extractManual(archive, dst string) error {
	f, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return errors.New("restore: the archive has no tock.db")
		}
		if err != nil {
			return fmt.Errorf("restore: %w", err)
		}
		if h.Name != "tock.db" {
			continue
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("restore: %w", err)
		}
		_, err = io.Copy(out, tr)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("restore: %w", err)
		}
		res, err := snapshot.Check(dst)
		if err != nil {
			return err
		}
		if res != "ok" {
			return &snapshot.DamagedError{Detail: res}
		}
		return nil
	}
}
