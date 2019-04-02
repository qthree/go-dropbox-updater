package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	progressbar "github.com/schollz/progressbar"
	dropbox "github.com/tj/go-dropbox"
	dropy "github.com/tj/go-dropy"
)

var remoteDirs = map[string]os.FileInfo{}
var remoteFiles = map[string]os.FileInfo{}
var localMissingFiles = []string{}
var localExpiredFiles = []string{}

// ScanRemote creates an index of dropbox source
func ScanRemote(client *dropy.Client, path string) {
	files, err := client.List(path)
	if err != nil {
		log.Panic(err)
	}
	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			remoteDirs[path+fileInfo.Name()] = fileInfo
			ScanRemote(client, path+fileInfo.Name()+"/")
		} else {
			remoteFiles[path+fileInfo.Name()] = fileInfo
		}
	}
}

func main() {
	download()
}

func download() {
	log.SetFlags(0)

	log.Println("Connecting to the update server...")
	client := dropy.New(dropbox.New(dropbox.NewConfig("YOUR_CLIENT_TOKEN_HERE!!!")))
	log.Println("Building file index, please wait...\n")
	ScanRemote(client, "/")
	log.Println("Found", len(remoteFiles), "remote files, checking for need in update...")

	missingSize := int(0)
	expiredSize := int(0)
	scanBar := progressbar.New(len(remoteFiles))
	for path, fileInfo := range remoteFiles {
		localInfo, err := os.Stat("." + path)
		if err != nil {
			localMissingFiles = append(localMissingFiles, path)
			missingSize += int(fileInfo.Size())
		} else {
			diff := localInfo.ModTime().Sub(fileInfo.ModTime())
			// Check if fileModTime differs (local is older) or size mismatch exist
			if localInfo.Size() != fileInfo.Size() || diff < (time.Duration(0)*time.Second) {
				localExpiredFiles = append(localExpiredFiles, path)
				expiredSize += int(fileInfo.Size())
			}
		}
		scanBar.Add(1)
	}
	fmt.Println(" Complete!\n")

	localMissingFilesCount := len(localMissingFiles)
	if localMissingFilesCount > 0 {
		log.Println("Downloading", localMissingFilesCount, "missing files,", missingSize, "bytes in total...")
		scanBar = progressbar.NewOptions(
			int(missingSize),
			progressbar.OptionSetBytes(int(missingSize)),
		)
		wg := sync.WaitGroup{}
		for _, path := range localMissingFiles {
			wg.Add(1)
			go func(path string) {
				inFile, err := client.Download(path)
				if err != nil {
					log.Panic(err)
				}

				err = os.MkdirAll("."+filepath.Dir(path), 0666)
				if err != nil {
					log.Panic(err)
				}

				var out io.Writer
				f, err := os.Create("." + path)
				if err != nil {
					log.Panic(err)
				}
				out = f
				defer f.Close()

				out = io.MultiWriter(out, scanBar)
				_, err = io.Copy(out, inFile)
				if err != nil {
					log.Panic(err)
				}
				wg.Done()
			}(path)
		}
		wg.Wait()
		fmt.Println(" Complete!\n")
	}
	localExpiredFilesCount := len(localExpiredFiles)
	if localExpiredFilesCount > 0 {
		log.Println("Overriding", localExpiredFilesCount, "expired files,", expiredSize, "bytes in total...")
		scanBar = progressbar.NewOptions(
			int(expiredSize),
			progressbar.OptionSetBytes(int(expiredSize)),
		)
		wg := sync.WaitGroup{}
		for _, path := range localExpiredFiles {
			wg.Add(1)
			go func(path string) {
				inFile, err := client.Download(path)
				if err != nil {
					log.Panic(err)
				}

				err = os.Remove("." + path)
				if err != nil {
					log.Panic(err)
				}

				var out io.Writer
				f, err := os.Create("." + path)
				if err != nil {
					log.Panic(err)
				}
				defer f.Close()
				out = f

				out = io.MultiWriter(out, scanBar)
				_, err = io.Copy(out, inFile)
				if err != nil {
					log.Panic(err)
				}

				wg.Done()
			}(path)
		}
		wg.Wait()
		fmt.Println(" Complete\n")
	}
	log.Println("Everything is up to date!")

	fmt.Print("Press 'Enter' to exit...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}
