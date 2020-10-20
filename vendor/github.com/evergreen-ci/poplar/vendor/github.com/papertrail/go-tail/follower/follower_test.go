package follower

import (
	"fmt"
	"log"
	"io"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	tmpDir string

	testLines = [][]string{
		{
			"’Twas brillig, and the slithy toves",
			"      Did gyre and gimble in the wabe:",
			"All mimsy were the borogoves,",
			"      And the mome raths outgrabe.",

			"“Beware the Jabberwock, my son!",
			"      The jaws that bite, the claws that catch!",
			"Beware the Jubjub bird, and shun",
			"      The frumious Bandersnatch!”",

			"He took his vorpal sword in hand;",
			"      Long time the manxome foe he sought—",
			"So rested he by the Tumtum tree",
			"      And stood awhile in thought.",

			"And, as in uffish thought he stood,",
			"      The Jabberwock, with eyes of flame,",
			"Came whiffling through the tulgey wood,",
			"      And burbled as it came!",

			"One, two! One, two! And through and through",
			"      The vorpal blade went snicker-snack!",
			"He left it dead, and with its head",
			"      He went galumphing back.",

			"“And hast thou slain the Jabberwock?",
			"      Come to my arms, my beamish boy!",
			"O frabjous day! Callooh! Callay!”",
			"      He chortled in his joy.",

			"’Twas brillig, and the slithy toves",
			"      Did gyre and gimble in the wabe:",
			"All mimsy were the borogoves,",
			"      And the mome raths outgrabe.",
		},

		{
			"The winter evening settles down",
			"With smell of steaks in passageways.",
			"Six o’clock.",
			"The burnt-out ends of smoky days.",
			"And now a gusty shower wraps",
			"The grimy scraps",
			"Of withered leaves about your feet",
			"And newspapers from vacant lots;",
			"The showers beat",
			"On broken blinds and chimney-pots,",
			"And at the corner of the street",
			"A lonely cab-horse steams and stamps.",
			"And then the lighting of the lamps.",
		},

		{
			"In Xanadu did Kubla Khan",
			"A stately pleasure-dome decree:",
			"Where Alph, the sacred river, ran",
			"Through caverns measureless to man",
			"   Down to a sunless sea.",
			"So twice five miles of fertile ground",
			"With walls and towers were girdled round;",
			"And there were gardens bright with sinuous rills,",
			"Where blossomed many an incense-bearing tree;",
			"And here were forests ancient as the hills,",
			"Enfolding sunny spots of greenery.",
		},
	}
)

func TestMain(m *testing.M) {
	tmpDir, _ = ioutil.TempDir("", "fllw")
	rs := m.Run()
	os.RemoveAll(tmpDir)
	if rs == 0 {
		// Followers may take 10 seconds to notice the removal.
		time.Sleep(10 * time.Second)
		if runtime.NumGoroutine() > 2 {
			// Heuristic to detect leaked goroutines.
			fmt.Println("--- FAIL: TestMain")
			logger := log.New(os.Stdout, "\t", log.Lshortfile)
			logger.Fatal("possible goroutine leak")
		}
	}
	os.Exit(rs)
}

func TestNoReopen(t *testing.T) {
	file, f := testPair(t, "TestNoReopen")
	defer file.Close()

	if err := writeLines(file, testLines[0]); err != nil {
		t.Fatal(err)
	}

	assertFollowedLines(t, f, testLines[0])
}

func TestTruncate(t *testing.T) {
	file, f := testPair(t, "TestTruncate")
	defer file.Close()

	if err := writeLines(file, testLines[0]); err != nil {
		t.Fatal(err)
	}

	assertFollowedLines(t, f, testLines[0])

	// normal copytruncate behavior
	if err := os.Truncate(file.Name(), 0); err != nil {
		t.Fatal(err)
	}

	// write a different set of lines
	if err := writeLines(file, testLines[1]); err != nil {
		t.Fatal(err)
	}

	assertFollowedLines(t, f, testLines[1])

	if err := os.Truncate(file.Name(), 0); err != nil {
		t.Fatal(err)
	}

	// imitate the behavior of a log writer with a handle that is
	// not O_APPEND, and has seeked past the end of the file
	file2, err := os.OpenFile(file.Name(), os.O_WRONLY, 0666)
	if err != nil {
		t.Fatal(err)
	}
	defer file2.Close()

	if _, err := file2.Seek(int64(1024*100*100), io.SeekStart); err != nil {
		t.Fatal(err)
	}

	// write a different set of lines
	if err := writeLines(file2, testLines[2]); err != nil {
		t.Fatal(err)
	}

	assertFollowedLines(t, f, testLines[2])
}

func TestRenameCreate(t *testing.T) {
	file, f := testPair(t, "TestRenameCreate")
	defer file.Close()

	if err := writeLines(file, testLines[0]); err != nil {
		t.Fatal(err)
	}

	assertFollowedLines(t, f, testLines[0])

	oldName := file.Name()
	if err := os.Rename(oldName, oldName+".1"); err != nil {
		t.Fatal(err)
	}

	newFile, err := os.Create(oldName)
	if err != nil {
		t.Fatal(err)
	}
	defer newFile.Close()

	// write a different set of lines
	if err := writeLines(newFile, testLines[1]); err != nil {
		t.Fatal(err)
	}

	assertFollowedLines(t, f, testLines[1])
}

func TestSymlink(t *testing.T) {
	// point symlink to first file
	testSymlink(t, "TestSymlink1", "TestSymlink")

	file, f := testPair(t, "TestSymlink")
	defer file.Close()

	if err := writeLines(file, testLines[0]); err != nil {
		t.Fatal(err)
	}

	assertFollowedLines(t, f, testLines[0])

	// now switch symlink to second file
	testSymlink(t, "TestSymlink2", "TestSymlink")

	newFile := testFile(t, "TestSymlink")
	defer newFile.Close()

	// write a different set of lines
	if err := writeLines(newFile, testLines[1]); err != nil {
		t.Fatal(err)
	}

	assertFollowedLines(t, f, testLines[1])
}

func testFile(t *testing.T, name string) *os.File {
	// open in append mode since most loggers will be doing such
	file, err := os.OpenFile(path.Join(tmpDir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func testSymlink(t *testing.T, oldname, newname string) {
	// unlink the target first, as with "ln -f"
	os.Remove(path.Join(tmpDir, newname))
	if err := os.Symlink(path.Join(tmpDir, oldname), path.Join(tmpDir, newname)); err != nil {
		t.Fatal(err)
	}
}

func testPair(t *testing.T, filename string) (*os.File, *Follower) {
	file := testFile(t, filename)

	f, err := New(file.Name(), Config{
		Reopen: true,
		Offset: 0,
		Whence: io.SeekEnd,
	})
	if err != nil {
		t.Fatal(err)
	}

	return file, f
}

func writeLines(file *os.File, lines []string) error {
	// sleep to make sure we don't write before the follower is ready
	time.Sleep(100 * time.Millisecond)

	for _, l := range lines {
		_, err := file.WriteString(l + "\n")
		if err != nil {
			return err
		}
	}

	return nil
}

func assertFollowedLines(t *testing.T, f *Follower, lines []string) {
	assert := assert.New(t)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		i := 0
		for line := range f.Lines() {
			assert.Equal(lines[i], line.String())
			i++
			if i == len(lines) {
				return
			}
		}
	}()

	if err := f.Err(); err != nil {
		t.Fatal(err)
	}

	wg.Wait()
}
