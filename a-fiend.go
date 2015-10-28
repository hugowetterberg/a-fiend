package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"time"

	"github.com/boltdb/bolt"
)

// Bucket name set
var (
	aliasBucket    = []byte{'a'}
	settingsBucket = []byte{'s'}
)

type aliasInfo struct {
	Name    string
	Command string
	Added   time.Time
	Labels  []string
}

func main() {
	dbPath := path.Join(os.Getenv("HOME"), ".a-fiend/alias.db")
	db, err := openDatabase(dbPath)

	if err != nil {
		log.Fatalf("Failed to open alias database for a-fiend: %s", err)
	}

	cmd := "list"
	args := []string{}
	if len(os.Args) > 1 {
		cmd = os.Args[1]
		args = os.Args[2:]
	}

	switch cmd {
	case "add":
		addAlias(db, args)
	case "list":
		listAliases(db, args)
	case "delete":
		deleteAlias(db, args)
	case "source":
		sourceFile(db, args)
	default:
		log.Printf("Unknown command %q", cmd)
		os.Exit(1)
	}

	db.Close()
}

func printSource(db *bolt.DB, arguments []string) {
	log.Printf("Print stuff: %#v", arguments)
}

func addAlias(db *bolt.DB, arguments []string) {
	alias := arguments[0]
	info := aliasInfo{
		Command: arguments[1],
		Added:   time.Now(),
	}
	data, err := json.Marshal(&info)
	if err != nil {
		log.Fatalf("Failed to marshal alias data for storage: %s", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(aliasBucket)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(alias), data)
	})
	if err != nil {
		log.Fatalf("Failed to save the alias: %s", err)
	}
	fmt.Printf("Successfully saved the alias %s=%q\n", alias, info.Command)
	updateSourceFile(db)
}

func deleteAlias(db *bolt.DB, arguments []string) {
	alias := arguments[0]

	err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(aliasBucket)
		if err != nil {
			return err
		}
		return bucket.Delete([]byte(alias))
	})
	if err != nil {
		log.Fatalf("Failed to delete the alias %s: %s", alias, err)
	}

	fmt.Printf("Successfully deleted the alias %q\n", alias)
	updateSourceFile(db)
}

func listAliases(db *bolt.DB, arguments []string) {
	err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(aliasBucket)
		if err != nil {
			return err
		}

		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			info := aliasInfo{}
			err = json.Unmarshal(v, &info)
			if err != nil {
				return err
			}

			fmt.Printf("alias %s=%s\n", string(k), quoteWith(info.Command, '\'', false))
		}

		return nil
	})
	if err != nil {
		log.Fatalf("Failed to read alias database: %s", err)
	}
}

func sourceFile(db *bolt.DB, arguments []string) {
	defaultDuration, err := getReminderDuration(db)
	if err != nil {
		log.Fatalf("Failed to get reminder duration setting: %s", err)
	}

	flags := flag.NewFlagSet("a-fiend source", flag.ExitOnError)

	silent := flags.Bool("silent", false, "Don't print source to stdout")
	reminders := flags.Duration("reminders", defaultDuration, "Duration to remind the user of new aliases")

	flags.Parse(arguments)

	if *reminders != defaultDuration {
		err := setReminderDuration(db, *reminders)
		if err != nil {
			log.Fatalf("Failed to update reminder duration setting: %s", err)
		}
	}

	file, err := os.Create(path.Join(os.Getenv("HOME"), ".a-fiend/source.sh"))
	if err != nil {
		log.Fatalf("Failed to create source file: %s", err)
	}
	defer file.Close()

	var writer io.Writer = file
	if !*silent {
		writer = io.MultiWriter(os.Stdout, file)
	}

	err = generateSourceFile(db, writer, *reminders, false)
	if err != nil {
		log.Fatalf("Failed to read alias database: %s", err)
	}
}

func updateSourceFile(db *bolt.DB) {
	defaultDuration, err := getReminderDuration(db)
	if err != nil {
		log.Fatalf("Failed to get reminder duration setting: %s", err)
	}

	file, err := os.Create(path.Join(os.Getenv("HOME"), ".a-fiend/source.sh"))
	if err != nil {
		log.Fatalf("Failed to create source file: %s", err)
	}
	defer file.Close()

	err = generateSourceFile(db, file, defaultDuration, false)
	if err != nil {
		log.Fatalf("Failed to read alias database: %s", err)
	}
}

func setReminderDuration(db *bolt.DB, duration time.Duration) error {
	return db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(settingsBucket)
		if err != nil {
			return err
		}
		return bucket.Put([]byte("reminders"), []byte(duration.String()))
	})
}

func getReminderDuration(db *bolt.DB) (time.Duration, error) {
	var duration time.Duration
	err := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(settingsBucket)
		if err != nil {
			return err
		}

		data := bucket.Get([]byte("reminders"))
		if len(data) == 0 {
			duration = time.Hour * 168 // One week
			return nil
		}

		parsed, err := time.ParseDuration(string(data))
		if err != nil {
			return err
		}
		duration = parsed
		return nil
	})
	return duration, err
}

func generateSourceFile(db *bolt.DB, w io.Writer, reminders time.Duration, regen bool) error {
	return db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(aliasBucket)
		if err != nil {
			return err
		}

		fmt.Fprintln(w, "# This file is automatically generated by a-fiend, do not edit")
		fmt.Fprintf(w, "# UPDATED %s\n", time.Now())
		fmt.Fprintf(w, "# Prints reminders for %s\n", reminders)
		fmt.Fprintln(w, "AFIEND_T=`date +%s`")

		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			info := aliasInfo{}
			err = json.Unmarshal(v, &info)
			if err != nil {
				return err
			}

			fmt.Fprintf(w, "alias %s=%s\n", string(k), quoteWith(info.Command, '\'', false))
			cutoff := info.Added.Add(reminders)
			fmt.Fprintf(w, "if [ %d -ge $AFIEND_T ]; then\n", cutoff.Unix())
			fmt.Fprintf(w, "\techo alias %s=%s\n", string(k), quoteWith(info.Command, '\'', false))
			fmt.Fprintln(w, "fi")
		}

		if regen {
			fmt.Fprintf(w, "(a-fiend source -reminders %s > /dev/null 2>&1 &)\n", reminders)
		}

		return nil
	})
}

func openDatabase(dbPath string) (*bolt.DB, error) {
	err := os.MkdirAll(path.Dir(dbPath), 0770)
	if err != nil {
		return nil, err
	}

	return bolt.Open(dbPath, 0660, bolt.DefaultOptions)
}
