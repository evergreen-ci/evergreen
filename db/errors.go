package db

import mgo "gopkg.in/mgo.v2"

func IsDuplicateKey(err error) bool  { return mgo.IsDup(err) }
func ResultsNotFound(err error) bool { return err == mgo.ErrNotFound }
