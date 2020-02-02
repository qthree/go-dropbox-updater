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

type FileEntry struct {
	Info os.FileInfo
	Location int
}
type PathEntry struct {
	Path string
	Location int
}

//var remoteDirs = map[string]os.FileInfo{}
//var remoteFiles = map[string]os.FileInfo{}
var remoteFiles = map[string]FileEntry{}
var localMissingFiles = []PathEntry{}
var localExpiredFiles = []PathEntry{}

// ScanRemote creates an index of dropbox source
func ScanRemote(client *dropy.Client, path string, location int) {
	files, err := client.List(path)
	if err != nil {
		log.Panic(err)
	}
	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			//remoteDirs[path+fileInfo.Name()] = fileInfo
			ScanRemote(client, path+fileInfo.Name()+"/", location)
		} else {
			remoteFiles[path+fileInfo.Name()] = FileEntry{fileInfo, location}
		}
	}
}

func main() {
	download()
}

func download() {
	log.SetFlags(0)

	log.Println("Connecting to the update server...")
	clients := []*dropy.Client{
		dropy.New(dropbox.New(dropbox.NewConfig("first key"))),
		dropy.New(dropbox.New(dropbox.NewConfig("second key"))),
	}
	log.Println("Building file index, please wait...\n")
	for location, client := range clients {
		ScanRemote(client, "/", location)
	}
	log.Println("Found", len(remoteFiles), "remote files, checking for need in update...")

	missingSize64 := int64(0)
	expiredSize64 := int64(0)
	scanBar := progressbar.New(len(remoteFiles))
	for path, entry := range remoteFiles {
		localInfo, err := os.Stat("." + path)
		if err != nil {
			localMissingFiles = append(localMissingFiles, PathEntry{path, entry.Location})
			missingSize64 += int64(entry.Info.Size())
		} else {
			diff := localInfo.ModTime().Sub(entry.Info.ModTime())
			// Check if fileModTime differs (local is older) or size mismatch exist
			if diff < (time.Duration(0)*time.Second) /*|| localInfo.Size() < fileInfo.Size()*/ {
				localExpiredFiles = append(localExpiredFiles, PathEntry{path, entry.Location})
				expiredSize64 += int64(entry.Info.Size())
			}
		}
		scanBar.Add(1)
	}
	fmt.Println(" Complete!\n")

	missingSize := int(missingSize64/1000)
	expiredSize := int(expiredSize64/1000)

	localMissingFilesCount := len(localMissingFiles)
	if localMissingFilesCount > 0 {
		log.Println("Downloading", localMissingFilesCount, "missing files,", missingSize, "kilobytes in total...")
		scanBar = progressbar.NewOptions(missingSize, progressbar.OptionSetRenderBlankState(true))
		wg := sync.WaitGroup{}
		for _, entry := range localMissingFiles {
			wg.Add(1)
			go func(path string, location int) {
				defer wg.Done()

				inFile, err := clients[location].Download(path)
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

				if fileEntry, ok := remoteFiles[path]; ok {
					err = os.Chtimes("."+path, fileEntry.Info.ModTime(), fileEntry.Info.ModTime())
					if err != nil {
						log.Println(err)
						return
					}
					scanBar.Add(int(fileEntry.Info.Size()/1000))
				}
			}(entry.Path, entry.Location)
		}
		wg.Wait()
		fmt.Println(" Complete!\n")
	}
	localExpiredFilesCount := len(localExpiredFiles)
	if localExpiredFilesCount > 0 {
		log.Println("Overriding", localExpiredFilesCount, "expired files,", expiredSize, "kilobytes in total...")
		scanBar = progressbar.NewOptions(expiredSize, progressbar.OptionSetRenderBlankState(true))
		wg := sync.WaitGroup{}
		for _, entry := range localExpiredFiles {
			wg.Add(1)
			go func(path string, location int) {
				defer wg.Done()

				inFile, err := clients[location].Download(path)
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

				if fileEntry, ok := remoteFiles[path]; ok {
					err = os.Chtimes("."+path, fileEntry.Info.ModTime(), fileEntry.Info.ModTime())
					if err != nil {
						log.Println(err)
						return
					}
					scanBar.Add(int(fileEntry.Info.Size()/1000))
				}
			}(entry.Path, entry.Location)
		}
		wg.Wait()
		fmt.Println(" Complete\n")
	}

	log.Println("Everything is up to date!")
	fmt.Print("Press 'Enter' to exit...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}
