package main_test

import (
	"math/rand"
	"testing"
	"time"
)

//func BenchmarkSomething(b *testing.B) {
//	b.Log("start")
//	for i := 0; i < 100; i++ {
//		time.Sleep(time.Duration(rand.Intn(500)))
//	}
//	time.Sleep(10 * time.Second)
//	b.Log("end")
//}
//
//func BenchmarkSomethingElse(b *testing.B) {
//	b.Log("start")
//	for i := 0; i < 100; i++ {
//		time.Sleep(time.Duration(rand.Intn(500)))
//	}
//	time.Sleep(10 * time.Second)
//	b.Log("end")
//}

func BenchmarkSomethingSilent(b *testing.B) {
	for i := 0; i < 100; i++ {
		time.Sleep(time.Duration(rand.Intn(500)))
	}
	time.Sleep(10 * time.Second)
}

//func BenchmarkSomethingFailing(b *testing.B) {
//	b.Log("start")
//	time.Sleep(10 * time.Second)
//	for i := 0; i < 100; i++ {
//		time.Sleep(time.Duration(rand.Intn(500)))
//	}
//
//	b.Error("deliberately failing")
//	b.Log("end")
//}
//
//func BenchmarkSomethingSkipped(b *testing.B) {
//	b.Log("start")
//	b.Skip("reasons")
//	time.Sleep(10 * time.Second)
//	for i := 0; i < 100; i++ {
//		time.Sleep(time.Duration(rand.Intn(500)))
//	}
//
//	b.Error("deliberately failing")
//	b.Log("end")
//}
//
//func BenchmarkSomethingSkippedSilent(b *testing.B) {
//	b.Skip("reasons")
//
//	time.Sleep(10 * time.Second)
//	for i := 0; i < 100; i++ {
//		time.Sleep(time.Duration(rand.Intn(500)))
//	}
//}
//
//func TestSomething(t *testing.T) {
//	t.Log("hi")
//}
