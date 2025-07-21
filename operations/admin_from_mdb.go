package operations

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func toMdbForLocal() cli.Command {
	const (
		dbFlagName           = "db"
		evergreenLocalDBName = "evergreen_local"
		inputFlagName        = "input"
		urlFlagName          = "url"
	)
	return cli.Command{
		Name:  "restore",
		Usage: "restore file produced by dump for a local Evergreen",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  urlFlagName,
				Usage: "specify the MongoDB URL",
				Value: "mongodb://127.0.0.1:27017",
			},
			cli.StringFlag{
				Name:  dbFlagName,
				Usage: "write to this database",
				Value: evergreenLocalDBName,
			},
			cli.StringFlag{
				Name:  inputFlagName,
				Usage: "read data from this file",
			},
		},
		Before: mergeBeforeFuncs(
			func(c *cli.Context) error {
				if c.String(inputFlagName) == "" {
					return errors.New("must specify an input file")
				}
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			dbName := c.String(dbFlagName)
			url := c.String(urlFlagName)
			infn := c.String(inputFlagName)

			client, err := mongo.Connect(ctx, options.Client().ApplyURI(url))
			if err != nil {
				return errors.Wrap(err, "creating MongoDB client")
			}

			f, err := os.Open(infn)
			if err != nil {
				return errors.Wrapf(err, "opening file '%s'", infn)
			}
			defer f.Close()

			gr, err := gzip.NewReader(f)
			if err != nil {
				return errors.Wrap(err, "creating gzip reader")
			}

			tr := tar.NewReader(gr)
			for {
				header, err := tr.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					return errors.Wrap(err, "iterating tar reader")
				}

				coll := client.Database(dbName).Collection(header.Name)
				size, err := coll.CountDocuments(ctx, struct{}{})
				if err != nil {
					return errors.Wrap(err, "finding number of source documents")
				}
				if size > 0 && dbName != evergreenLocalDBName {
					return errors.Errorf("looks like there are already documents in collection '%s', and it's not in the database '%s', exiting for safety", header.Name, evergreenLocalDBName)
				}

				var buf *bytes.Buffer
				switch header.Typeflag {
				case tar.TypeDir:
					continue
				case tar.TypeReg:
					buf = &bytes.Buffer{}
					_, _ = io.Copy(buf, tr)
				}

				docs := []any{}
				for {
					doc := &birch.Document{}
					if _, err := doc.ReadFrom(buf); err != nil {
						if err == io.EOF {
							break
						}
						return errors.Wrap(err, "reading document from buffer")
					}
					docs = append(docs, doc)
				}
				_, _ = coll.InsertMany(ctx, docs)
				grip.Infof("inserted %d docs into collection '%s'", len(docs), header.Name)
			}

			return nil
		},
	}
}

func fromMdbForLocal() cli.Command {
	const (
		dbFlagName      = "db"
		outputFlagName  = "output"
		projectFlagName = "project"
		urlFlagName     = "url"
	)
	collections := []string{"versions", "builds", "tasks", "distro", "project_ref"}
	return cli.Command{
		Name:  "dump",
		Usage: "dump a portion of a MongoDB database for a local Evergreen",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  projectFlagName,
				Usage: "get data for only this project, to limit data dump",
			},
			cli.StringFlag{
				Name:  outputFlagName,
				Usage: "write data to this file",
			},
			cli.StringFlag{
				Name:  urlFlagName,
				Usage: "specify the MongoDB URL",
				Value: "mongodb://127.0.0.1:27017",
			},
			cli.StringFlag{
				Name:  dbFlagName,
				Usage: "read data from this database",
				Value: "mci",
			},
		},
		Before: mergeBeforeFuncs(
			func(c *cli.Context) error {
				if c.String(outputFlagName) == "" {
					return errors.New("must specify an output file")
				}
				if c.String(projectFlagName) == "" {
					return errors.New("must specify a project to dump data for")
				}
				return nil
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			outfn := c.String(outputFlagName)
			project := c.String(projectFlagName)
			dbName := c.String(dbFlagName)
			url := c.String(urlFlagName)

			var filters map[string]bson.M
			if project != "" {
				filters = map[string]bson.M{
					"versions": {"identifier": project},
					"builds":   {"branch": project},
					"tasks":    {"branch": project},
				}
			}

			client, err := mongo.Connect(ctx, options.Client().ApplyURI(url))
			if err != nil {
				return errors.Wrap(err, "creating MongoDB client")
			}

			for _, collection := range collections {
				coll := client.Database(dbName).Collection(collection)
				var size int64
				size, err = coll.CountDocuments(ctx, struct{}{})
				if err != nil {
					return errors.Wrap(err, "finding number of source documents")
				}
				if size == 0 {
					return errors.Errorf("cannot write data from collection '%s' without documents", collection)
				}
			}

			if _, err = os.Stat(outfn); !os.IsNotExist(err) {
				return errors.Errorf("cannot export to file '%s', file already exists", outfn)
			}

			f, err := os.Create(outfn)
			if err != nil {
				return errors.Wrapf(err, "opening file '%s'", outfn)
			}
			gw := gzip.NewWriter(f)
			defer gw.Close()
			tw := tar.NewWriter(gw)
			defer tw.Close()
			var collBuf *bytes.Buffer

			for _, collection := range collections {
				collBuf = &bytes.Buffer{}
				err := processCollection(ctx, collBuf, client, filters, dbName, collection)
				if err != nil {
					return err
				}

				hdr := &tar.Header{
					Name: collection,
					Mode: 0600,
					Size: int64(collBuf.Len()),
				}
				if err := tw.WriteHeader(hdr); err != nil {
					return errors.Wrap(err, "writing tar header")
				}
				if _, err := tw.Write(collBuf.Bytes()); err != nil {
					return errors.Wrap(err, "writing buffer to tarball")
				}

			}

			return f.Close()
		},
	}
}

func processCollection(ctx context.Context, collBuf *bytes.Buffer, client *mongo.Client, filters map[string]bson.M, dbName, collection string) error {
	var filter bson.M
	var ok bool

	coll := client.Database(dbName).Collection(collection)
	if filter, ok = filters[collection]; !ok {
		filter = bson.M{}
	}
	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		return errors.Wrap(err, "finding documents")
	}
	defer func() {
		grip.Error(cursor.Close(ctx))
	}()

	count := 0

	for cursor.Next(ctx) {
		_, err := collBuf.Write(cursor.Current)
		if err != nil {
			return errors.Wrap(err, "writing document")
		}
		count++
	}

	grip.Info(message.Fields{
		"count":      count,
		"collection": collection,
		"database":   dbName,
	})

	return nil
}
