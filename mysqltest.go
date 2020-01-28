// Spawns a MySQL server with a single database configured. Ideal for unit
// tests where you want a clean instance each time. Then clean up afterwards.
//
// Requires MySQL to be installed on your system (but it doesn't have to be running).
package mysqltest

import (
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MySQL struct {
	dir string
	cmd *exec.Cmd
	DB  *sql.DB

	stderr io.ReadCloser
	stdout io.ReadCloser

	isRoot   bool
	binPath  string
	sockFile string
}

// Start a new MySQL database, on temporary storage.
//
// Use the DB field to access the database connection
func Start() (*MySQL, error) {
	// Handle dropping permissions when running as root
	me, err := user.Current()
	if err != nil {
		return nil, err
	}
	isRoot := me.Username == "root"

	mysqlUID := int(0)
	mysqlGID := int(0)
	if isRoot {
		mysqlUser, err := user.Lookup("mysql")
		if err != nil {
			return nil, fmt.Errorf("Could not find mysql user, which is required when running as root: %s", err)
		}

		uid, err := strconv.ParseInt(mysqlUser.Uid, 10, 64)
		if err != nil {
			return nil, err
		}
		mysqlUID = int(uid)

		gid, err := strconv.ParseInt(mysqlUser.Gid, 10, 64)
		if err != nil {
			return nil, err
		}
		mysqlGID = int(gid)
	}

	// Prepare data directory
	dir, err := ioutil.TempDir("", "mysqltest")
	if err != nil {
		return nil, err
	}

	dataDir := path.Join(dir, "data")
	sockDir := path.Join(dir, "sock")
	sockFile := path.Join(sockDir, "mysql.sock")

	err = os.MkdirAll(dataDir, 0711)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(sockDir, 0711)
	if err != nil {
		return nil, err
	}

	if isRoot {
		err = os.Chmod(dir, 0711)
		if err != nil {
			return nil, err
		}

		err = os.Chown(dataDir, mysqlUID, mysqlGID)
		if err != nil {
			return nil, err
		}

		err = os.Chown(sockDir, mysqlUID, mysqlGID)
		if err != nil {
			return nil, err
		}
	}

	// Write config file
	configFile := path.Join(dir, "my.cnf")
	err = ioutil.WriteFile(configFile, []byte(fmt.Sprintf(`[mysqld]
datadir = %s
socket = %s/mysql.sock
general_log_file = %s/out.log
general_log = 1
skip-networking
`, dataDir, sockDir, dir)), 0644)

	// Find executables root path
	binPath, err := findBinPath()
	if err != nil {
		return nil, err
	}

	// Figure out what we are running
	version := prepareCommand(isRoot, path.Join(binPath, "mysql"),
		"--version",
	)
	out, err := version.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Failed to get version: %w -> %s", err, string(out))
	}
	isMariaDB := strings.Contains(string(out), "MariaDB")

	// Initialize MySQL data directory
	if isMariaDB {
		init := prepareCommand(isRoot, path.Join(binPath, "mysql_install_db"),
			fmt.Sprintf("--datadir=%s", dataDir),
		)
		out, err = init.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("Failed to initialize DB: %w -> %s", err, string(out))
		}
	} else {
		init := prepareCommand(isRoot, path.Join(binPath, "mysqld_safe"),
			"--initialize-insecure",
			fmt.Sprintf("--datadir=%s", dataDir),
		)
		out, err = init.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("Failed to initialize DB: %w -> %s", err, string(out))
		}
	}

	// Start MySQL
	cmd := prepareCommand(isRoot, path.Join(binPath, "mysqld_safe"),
		fmt.Sprintf("--defaults-file=%s", configFile),
	)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stderr.Close()
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, abort("Failed to start database", cmd, stderr, stdout, err)
	}

	mysql := &MySQL{
		cmd: cmd,
		dir: dir,

		stderr: stderr,
		stdout: stdout,

		isRoot:   isRoot,
		binPath:  binPath,
		sockFile: sockFile,
	}

	// Connect to DB, waiting for it to start
	err = retry(func() error {
		dsn := makeDSN(sockFile, "test")
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return err
		}

		err = db.Ping()
		if err != nil {
			return err
		}

		mysql.DB = db
		return nil
	}, 1000, 10*time.Millisecond)
	if err != nil {
		return nil, abort("Failed to connect to test DB", cmd, stderr, stdout, err)
	}

	return mysql, nil
}

// Stop the database and remove storage files.
func (p *MySQL) Stop() error {
	if p == nil {
		return nil
	}

	defer func() {
		// Always try to remove it
		os.RemoveAll(p.dir)
	}()

	// mysqladmin -u root -S /tmp/mysqltest810067242/sock/mysql.sock shutdown
	shutdown := prepareCommand(p.isRoot, path.Join(p.binPath, "mysqladmin"),
		"-u", "root",
		"-S", p.sockFile,
		"shutdown",
	)
	out, err := shutdown.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to shutdown DB: %w -> %s", err, string(out))
	}

	err = p.cmd.Wait()
	if err != nil {
		return err
	}

	if p.stderr != nil {
		p.stderr.Close()
	}

	if p.stdout != nil {
		p.stdout.Close()
	}

	return nil
}

// Needed because Ubuntu doesn't put initdb in $PATH
func findBinPath() (string, error) {
	// In $PATH (e.g. Fedora) great!
	p, err := exec.LookPath("mysqld_safe")
	if err == nil {
		return path.Dir(p), nil
	}

	return "", fmt.Errorf("Did not find MySQL / MariaDB executables installed")
}

func makeDSN(sockDir, dbname string) string {
	return fmt.Sprintf("root@unix(%s)/%s", sockDir, dbname)
}

func retry(fn func() error, attempts int, interval time.Duration) error {
	for {
		err := fn()
		if err == nil {
			return nil
		}

		attempts -= 1
		if attempts <= 0 {
			return err
		}

		time.Sleep(interval)
	}
}

func prepareCommand(isRoot bool, command string, args ...string) *exec.Cmd {
	if !isRoot {
		return exec.Command(command, args...)
	}

	for i, a := range args {
		if a == "" {
			args[i] = "''"
		}
	}

	return exec.Command("su",
		"-",
		"mysql",
		"-c",
		strings.Join(append([]string{command}, args...), " "),
	)
}

func abort(msg string, cmd *exec.Cmd, stderr, stdout io.ReadCloser, err error) error {
	cmd.Process.Signal(os.Interrupt)
	cmd.Wait()

	serr, _ := ioutil.ReadAll(stderr)
	sout, _ := ioutil.ReadAll(stdout)
	stderr.Close()
	stdout.Close()
	return fmt.Errorf("%s: %s\nOUT: %s\nERR: %s", msg, err, string(sout), string(serr))
}
