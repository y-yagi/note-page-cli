package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"github.com/y-yagi/configure"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type page struct {
	ID        string
	Content   string    `firestore:"content"`
	Name      string    `firestore:"name"`
	CreatedAt time.Time `firestore:"createdAt"`
	UpdatedAt time.Time `firestore:"updatedAt"`
}

type notebook struct {
	ID        string
	Name      string    `firestore:"name"`
	CreatedAt time.Time `firestore:"createdAt"`
	UpdatedAt time.Time `firestore:"updatedAt"`
}

const cmd = "note-page-cli"

type config struct {
	AccountKeyFile string `toml:"account_key_file"`
}

var cfg config
var ctx context.Context

func init() {
	if !configure.Exist(cmd) {
		cfg.AccountKeyFile = ""
		configure.Save(cmd, cfg)
	}
}

func main() {
	var edit bool
	var migrate bool

	flag.BoolVar(&edit, "c", false, "edit config")
	flag.BoolVar(&migrate, "m", false, "migrate Data")
	flag.Parse()

	if flag.NArg() != 0 {
		fmt.Printf("Usage: %s\n", cmd)
		flag.PrintDefaults()
		os.Exit(2)
	}

	if edit {
		if err := editConfig(); err != nil {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	err := configure.Load(cmd, &cfg)
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		os.Exit(1)
	}

	if cfg.AccountKeyFile == "" {
		fmt.Printf("please set key file to config file\n")
		os.Exit(1)
	}

	ctx = context.Background()
	client, err := generateClient()
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	if migrate {
		if err = addNoteBookId(client); err != nil {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}

		fmt.Printf("migrate finished.\n")
	} else {
		var books []notebook
		if err = fetchNoteBooks(client, &books); err != nil {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}

		fmt.Printf("%+v\n", books)

		var pages []page
		if err = fetchPages(client, &pages); err != nil {
			fmt.Printf("%v\n", err)
			os.Exit(1)
		}

		fmt.Printf("%+v\n", pages)
	}

	os.Exit(0)
}

func editConfig() error {
	editor := os.Getenv("EDITOR")
	if len(editor) == 0 {
		editor = "vim"
	}

	return configure.Edit(cmd, editor)
}

func generateClient() (*firestore.Client, error) {
	opt := option.WithCredentialsFile(cfg.AccountKeyFile)
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return nil, fmt.Errorf("error initializing app: %v", err)
	}
	client, err := app.Firestore(ctx)
	if err != nil {
		return nil, fmt.Errorf("error get client: %v", err)
	}

	return client, nil
}

func fetchPages(client *firestore.Client, pages *[]page) error {
	iter := client.Collection("pages").OrderBy("createdAt", firestore.Asc).Documents(ctx)

	var p page

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate: %v", err)
		}

		if err := doc.DataTo(&p); err != nil {
			return fmt.Errorf("failed to convert to Bookmark: %v", err)
		}
		p.ID = doc.Ref.ID

		*pages = append(*pages, p)
	}

	return nil
}

func fetchNoteBooks(client *firestore.Client, notebooks *[]notebook) error {
	iter := client.Collection("notebooks").OrderBy("createdAt", firestore.Asc).Documents(ctx)

	var b notebook

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate: %v", err)
		}

		if err := doc.DataTo(&b); err != nil {
			return fmt.Errorf("failed to convert to Bookmark: %v", err)
		}
		b.ID = doc.Ref.ID

		*notebooks = append(*notebooks, b)
	}

	return nil
}

func addNoteBookId(client *firestore.Client) error {
	var books []notebook
	if err := fetchNoteBooks(client, &books); err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}

	var defaultBook notebook
	for _, book := range books {
		if book.Name == "default" {
			defaultBook = book
			break
		}
	}

	iter := client.Collection("pages").Documents(ctx)

	return client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				return nil
			}

			if err != nil {
				return fmt.Errorf("failed to iterate: %v", err)
			}

			if id, _ := doc.DataAt("noteBookId"); id != nil {
				continue
			}

			if err = tx.Set(doc.Ref, map[string]interface{}{"noteBookId": defaultBook.ID}, firestore.MergeAll); err != nil {
				return err
			}
		}
	})
}
