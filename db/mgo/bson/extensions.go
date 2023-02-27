package bson

import "go.mongodb.org/mongo-driver/bson/bsontype"

func (m M) MarshalBSON() ([]byte, error)     { return Marshal(m) }
func (m M) UnmarshalBSON(in []byte) error    { return Unmarshal(in, m) }
func (d D) MarshalBSON() ([]byte, error)     { return Marshal(d) }
func (d D) UnmarshalBSON(in []byte) error    { return Unmarshal(in, &d) }
func (r RawD) MarshalBSON() ([]byte, error)  { return Marshal(r) }
func (r RawD) UnmarshalBSON(in []byte) error { return Unmarshal(in, &r) }

func (o *ObjectId) UnmarshalBSONValue(t bsontype.Type, in []byte) error {
	*o = ObjectId(in)
	return nil
}
func (o ObjectId) MarshalBSONValue() (bsontype.Type, []byte, error) {
	return bsontype.ObjectID, []byte(o), nil
}
