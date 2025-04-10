package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/user"
	"sync"
	"time"
)

var (
	lastUpdateTime  time.Time
	nextRunTime     time.Time
	retryCount      int
	lastFailureTime time.Time
	timer           *time.Timer
	mu              sync.Mutex
)

func main() {
	debug := flag.Bool("d", false, "Enable debug logging")
	period := flag.Int("t", 300, "Debounce period in seconds (min 180)")
	maxRetries := flag.Int("max-retry", 1, "Max retries on failure")
	retryPeriod := flag.Int("retry-period", 3600, "Seconds to reset retry count")
	hugoCmd := flag.String("hugo-cmd", "cd /home/ti/blog_/myblog && /home/nginX/bin/hugo --minify --noBuildLock --cleanDestinationDir", "Command to run Hugo")
	help := flag.Bool("h", false, "Show this help message")
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	if u, _ := user.Current(); u != nil && u.Uid == "0" {
		fmt.Println("Error: don't allow 'root' to run")
		os.Exit(1)
	}

	if *period < 180 {
		*period = 180
		log.Printf("Period adjusted to minimum: %ds", *period)
	}

	log.Printf("Listening on unix:/wwwFS.out/unix.hugo_update_daemon.sock and 0.0.0.0:45718, period: %ds, using command: %s", *period, *hugoCmd)

	go func() {
		os.Remove("/wwwFS.out/unix.hugo_update_daemon.sock")
		unixListener, err := net.Listen("unix", "/wwwFS.out/unix.hugo_update_daemon.sock")
		if err != nil {
			log.Fatalf("Failed to listen on Unix socket: %v", err)
		}
		defer unixListener.Close()
		if err := os.Chmod("/wwwFS.out/unix.hugo_update_daemon.sock", 0666); err != nil {
			log.Fatalf("Failed to set socket permissions: %v", err)
		}
		for {
			conn, err := unixListener.Accept()
			if err != nil {
				log.Printf("Unix accept error: %v", err)
				continue
			}
			go handleConnection(conn, "unix", *period, *maxRetries, *retryPeriod, *hugoCmd, *debug)
		}
	}()

	tcpListener, err := net.Listen("tcp", "0.0.0.0:45718")
	if err != nil {
		log.Fatalf("Failed to listen on TCP: %v", err)
	}
	defer tcpListener.Close()
	for {
		conn, err := tcpListener.Accept()
		if err != nil {
			log.Printf("TCP accept error: %v", err)
			continue
		}
		go handleConnection(conn, "tcp", *period, *maxRetries, *retryPeriod, *hugoCmd, *debug)
	}
}

func handleConnection(conn net.Conn, source string, period, maxRetries, retryPeriod int, hugoCmd string, debug bool) {
	conn.Close()

	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	if debug {
		log.Printf("Source: %s, last_update_time: %s, next_run_time: %s, retry_count: %d", source, lastUpdateTime, nextRunTime, retryCount)
	}

	if lastUpdateTime.IsZero() || now.Sub(lastUpdateTime) > time.Duration(period)*time.Second {
		if timer != nil {
			timer.Stop()
			timer = nil
		}
		runHugo(period, maxRetries, hugoCmd, debug)
	} else if nextRunTime.IsZero() {
		nextRunTime = lastUpdateTime.Add(time.Duration(period) * time.Second)
		timer = time.AfterFunc(time.Until(nextRunTime), func() {
			mu.Lock()
			defer mu.Unlock()
			runHugo(period, maxRetries, hugoCmd, debug)
		})
		if debug {
			log.Printf("Scheduled next run at: %s", nextRunTime)
		}
	}

	if !lastFailureTime.IsZero() && now.Sub(lastFailureTime) > time.Duration(retryPeriod)*time.Second {
		retryCount = 0
		lastFailureTime = time.Time{}
		if debug {
			log.Printf("Retry count reset after %ds", retryPeriod)
		}
	}
}

func runHugo(period, maxRetries int, hugoCmd string, debug bool) {
	cmd := exec.Command("/bin/sh", "-c", hugoCmd)
	if debug {
		log.Println("Before run")
	}
	err := cmd.Run()
	now := time.Now()
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
	}
	if debug {
		log.Printf("After run, exit code %d", exitCode)
	}

	if err != nil {
		if debug {
			log.Printf("Hugo failed: %v", err)
		}
		if retryCount < maxRetries {
			retryCount++
			lastFailureTime = now
			nextRunTime = now.Add(time.Duration(period) * time.Second)
			timer = time.AfterFunc(time.Until(nextRunTime), func() {
				mu.Lock()
				defer mu.Unlock()
				runHugo(period, maxRetries, hugoCmd, debug)
			})
			if debug {
				log.Printf("Retry scheduled at: %s, retry_count: %d", nextRunTime, retryCount)
			}
		} else if debug {
			log.Println("Max retries reached, check Hugo setup")
		}
	} else {
		lastUpdateTime = now
		nextRunTime = time.Time{}
		retryCount = 0
		if debug {
			log.Printf("Hugo ran successfully, last_update_time: %s", lastUpdateTime)
		}
	}
}
