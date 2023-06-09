package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var watcher *fsnotify.Watcher

type Configuration struct {
	DbType   string `json:"DbType"`
	Host     string `json:"Host"`
	Port     string `json:"Port"`
	DbUser   string `json:"DbUser"`
	DbPwd    string `json:"DbPwd"`
	DbName   string `json:"DbName"`
	FileColl string `json:"FileColl"`
	TreeColl string `json:"TreeColl"`
}

// StartWatcher starts the file watcher
func StartWatcher(path string) {
	// Create your file with desired read/write permissions
	f, err := os.OpenFile("tracelog.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// START Read JSON Config
	jsonData, err := ioutil.ReadFile("conf.json")
	if err != nil {
		fmt.Println(err)
		return
	}

	// Parse the JSON data
	var varConf Configuration
	err = json.Unmarshal(jsonData, &varConf)
	if err != nil {
		fmt.Println(err)
		return
	}
	// END Read JSON Config

	// Set output of logs to f
	log.SetOutput(f)

	// Creates a new file watcher
	watcher, _ = fsnotify.NewWatcher()
	defer watcher.Close()

	// Starting at the root of the project, walk each file/directory searching for directories
	if err := filepath.Walk(path, watchDir); err != nil {
		fmt.Println("ERROR", err)
	}

	// Declare host and port options to pass to the Connect() method
	mongodbURI := varConf.DbType + "://" + varConf.DbUser + ":" + varConf.DbPwd + "@" + varConf.Host + ":" + varConf.Port

	// Connect to MongoDB
	clientOptions := options.Client().ApplyURI(mongodbURI)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Println("mongo.Connect() ERROR:", err)
		os.Exit(1)
	}
	defer func() {
		err = client.Disconnect(context.TODO())
		if err != nil {
			log.Println("client.Disconnect() ERROR:", err)
			os.Exit(1)
		}
	}()

	// Access a MongoDB collection through a database
	collection := client.Database("sopie").Collection("watcher")

	done := make(chan bool)

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Create == fsnotify.Create {
					file, err := os.Stat(event.Name)
					if err != nil {
						log.Println("Error getting file info:", err)
						continue
					}
					if !file.IsDir() {
						fileInfo := struct {
							Name string `json:"Name"`
							Date string `json:"Date"`
						}{
							Name: event.Name,
							Date: file.ModTime().String(),
						}
						fileInfoJSON, err := json.Marshal(fileInfo)
						if err != nil {
							log.Println("Error marshaling JSON:", err)
							continue
						}
						log.Println(string(fileInfoJSON))
						_, err = collection.InsertOne(context.TODO(), fileInfo)
						if err != nil {
							log.Println("Error inserting document into MongoDB:", err)
							continue
						}
					}
				}
			case err := <-watcher.Errors:
				log.Println("ERROR", err)
			}
		}
	}()

	<-done
}

// watchDir gets run as a walk func, searching for directories to add watchers to
func watchDir(path string, fi os.FileInfo, err error) error {
	// Since fsnotify can watch all the files in a directory, watchers only need
	// to be added to each nested directory
	if fi.Mode().IsDir() {
		return watcher.Add(path)
	}
	return nil
}
