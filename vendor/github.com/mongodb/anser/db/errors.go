package db

import mgo "gopkg.in/mgo.v2"

func ResultsNotFound(err error) bool { return err == mgo.ErrNotFound }
