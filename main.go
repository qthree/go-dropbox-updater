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
	client := dropy.New(dropbox.New(dropbox.NewConfig("YOUR_DROPBOX_APP_CLIENT_KEY_HERE")))
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
			if diff < (time.Duration(0)*time.Second) || localInfo.Size() < fileInfo.Size() {
				localExpiredFiles = append(localExpiredFiles, path)
				expiredSize += int(fileInfo.Size())
			}
		}
		scanBar.Add(1)
	}
	fmt.Println(" Complete!\n")

	missingSize = int(float32(missingSize) * (0.9))
	expiredSize = int(float32(expiredSize) * (0.9))

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
				defer wg.Done()

				inFile, err := client.Download(path)
				if err != nil {
					log.Println(err)
					return
				}

				err = os.MkdirAll("."+filepath.Dir(path), 0666)
				if err != nil {
					log.Println(err)
					return
				}

				var out io.Writer
				f, err := os.Create("." + path + ".foupdate")
				if err != nil {
					log.Println(err)
					return
				}
				out = f

				out = io.MultiWriter(out, scanBar)
				_, err = io.Copy(out, inFile)
				if err != nil {
					log.Println(err)
					return
				}

				f.Close()

				err = os.Rename("."+path+".foupdate", "."+path)
				if err != nil {
					log.Println(err)
					return
				}

				if fileInfo, ok := remoteFiles[path]; ok {
					err = os.Chtimes("."+path, fileInfo.ModTime(), fileInfo.ModTime())
					if err != nil {
						log.Println(err)
						return
					}
				}
			}(path)
		}
		wg.Wait()
		scanBar.Add(missingSize)
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
				defer wg.Done()

				inFile, err := client.Download(path)
				if err != nil {
					log.Println(err)
					return
				}

				var out io.Writer
				f, err := os.Create("." + path + ".foupdate")
				if err != nil {
					log.Println(err)
					return
				}
				out = f

				out = io.MultiWriter(out, scanBar)
				_, err = io.Copy(out, inFile)
				if err != nil {
					log.Println(err)
					return
				}

				err = os.Remove("." + path)
				if err != nil {
					log.Println(err)
					return
				}

				f.Close()

				err = os.Rename("."+path+".foupdate", "."+path)
				if err != nil {
					log.Println(err)
					return
				}

				if fileInfo, ok := remoteFiles[path]; ok {
					err = os.Chtimes("."+path, fileInfo.ModTime(), fileInfo.ModTime())
					if err != nil {
						log.Println(err)
						return
					}
				}
			}(path)
		}
		wg.Wait()
		scanBar.Add(expiredSize)
		fmt.Println(" Complete\n")
	}

	log.Println("Everything is up to date!")
	fmt.Print("Press 'Enter' to exit...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}
