package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	youtubedl, timidity, ffmpeg, sox string
	tmpdir                           string
	outdir                           string
)

func init() {
	var err error
	if youtubedl, err = exec.LookPath("youtube-dl"); err != nil {
		panic("unable to find youtube-dl")
	}
	if timidity, err = exec.LookPath("timidity"); err != nil {
		panic("unable to find timidity")
	}
	if ffmpeg, err = exec.LookPath("ffmpeg"); err != nil {
		panic("unable to find ffmpeg")
	}
	if sox, err = exec.LookPath("sox"); err != nil {
		panic("unable to find sox")
	}
	if tmpdir, err = ioutil.TempDir("", "quizrip_"); err != nil {
		panic("unable to create temp dir")
	}
	log.Println("Using", tmpdir)
}

type csvline []string

func (l csvline) Question() string {
	if len(l[1]) < 2 {
		return "0" + l[1]
	}
	return l[1]
}
func (l csvline) Type() string { return l[2] }
func (l csvline) Title() string {
	return strings.NewReplacer(" - ", "-", " ", "-").Replace(l[3]) + ".mp3"
}
func (l csvline) URL() []string   { return strings.Split(l[4], " | ") }
func (l csvline) QStart() string  { return l[5] }
func (l csvline) QLength() string { return l[6] }
func (l csvline) AStart() string  { return l[7] }
func (l csvline) ALength() string { return l[8] }
func (l csvline) Speed() string   { return l[9] }
func (l csvline) OverlapPath() string {
	return filepath.Join(
		outdir,
		"questions",
		fmt.Sprintf(
			"%s-overlap.mp3",
			l.Question(),
		),
	)
}
func (l csvline) Path(prefix string) string {
	return filepath.Join(
		outdir,
		prefix,
		fmt.Sprintf(
			"%s-%s",
			l.Question(),
			l.Title(),
		),
	)
}

var overlap = map[string][]csvline{}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: ./quizrip csvfile outputfolder")
		os.Exit(1)
	}

	lines, err := readCSV(os.Args[1])
	if err != nil {
		fmt.Println("unable to read input file:", err)
		os.Exit(1)
	}
	outdir = os.Args[2]
	if err := os.MkdirAll(outdir, 0700); err != nil {
		fmt.Println("unable to create output folder:", err)
		os.Exit(1)
	}
	os.MkdirAll(filepath.Join(outdir, "questions"), 0700)
	os.MkdirAll(filepath.Join(outdir, "reponses"), 0700)

	lines = lines[1:] //Skipping headers
	for _, l := range lines {
		line := csvline(l)
		switch line.Type() {
		case "overlap":
			overlap[line.Question()] = append(overlap[line.Question()], line)
			fallthrough
		case "normal":
			if err := processNormal(line); err != nil {
				fmt.Printf("unable to process %s:%s", line.Title(), err)
			}
		case "midi":
			if err := processMidi(line); err != nil {
				fmt.Printf("unable to process %s:%s", line.Title(), err)
			}
		case "slow", "fast":
			if err := processSpeed(line); err != nil {
				fmt.Printf("unable to process %s:%s", line.Title(), err)
			}
		}
	}
	if err := processOverlap(); err != nil {
		fmt.Printf("unable to process overlap:%s", err)
	}
}

func processOverlap() error {
	//loop through all overlap
	//loop through all song
	for q, lines := range overlap {
		if len(lines) != 2 {
			return fmt.Errorf("unable to process overlap with more/less than 2 songs for question", q)
		}
		mp3, err := mixMp3(lines[0].Path("questions"), lines[1].Path("questions"), "1.0")
		if err != nil {
			return err
		}
		os.Rename(mp3, lines[0].OverlapPath())
		os.Remove(lines[0].Path("questions"))
		os.Remove(lines[1].Path("questions"))
	}
	return nil
}

func processMidi(l csvline) error {

	midi, err := download(l.URL()[0])
	if err != nil {
		return err
	}

	mp3, err := convertMidiToMp3(midi)
	if err != nil {
		return err
	}

	question, err := cutMp3(mp3, l.QStart(), l.QLength())
	if err != nil {
		return err
	}

	answermp3, err := ripYoutubeMp3(l.URL()[1])
	if err != nil {
		return err
	}

	answer, err := cutMp3(answermp3, l.AStart(), l.ALength())
	if err != nil {
		return err
	}

	if err := os.Rename(question, l.Path("questions")); err != nil {
		return err
	}
	return os.Rename(answer, l.Path("reponses"))
}
func processSpeed(l csvline) error {
	if err := processNormal(l); err != nil {
		return err
	}
	mp3, err := changeSpeed(l.Path("questions"), l.Speed())
	if err != nil {
		return err
	}
	return os.Rename(mp3, l.Path("questions"))
}

func processNormal(l csvline) error {
	mp3, err := ripYoutubeMp3(l.URL()[0])
	if err != nil {
		return err
	}

	question, err := cutMp3(mp3, l.QStart(), l.QLength())
	if err != nil {
		return err
	}
	answer, err := cutMp3(mp3, l.AStart(), l.ALength())
	if err != nil {
		return err
	}

	if err := os.Rename(question, l.Path("questions")); err != nil {
		return err
	}
	return os.Rename(answer, l.Path("reponses"))
}

func readCSV(infile string) ([][]string, error) {
	f, err := os.Open(infile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	return r.ReadAll()
}

func tempfile() *os.File {
	f, err := ioutil.TempFile(tmpdir, "")
	if err != nil {
		panic(err)
	}
	return f
}

func tempname(sufix string) string {
	f := tempfile()
	f.Close()
	os.Remove(f.Name())
	return f.Name() + sufix
}

func tempdir() string {
	dir, err := ioutil.TempDir(tmpdir, "")
	if err != nil {
		panic(err)
	}
	return dir
}

func command(name string, args ...string) *exec.Cmd {
	log.Printf("Running %s %s", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	// cmd.Stderr = os.Stderr
	// cmd.Stdout = os.Stdout
	return cmd
}

func download(url string) (outfile string, err error) {
	var resp *http.Response
	if resp, err = http.Get(url); err != nil {
		return
	}
	defer resp.Body.Close()

	f := tempfile()
	defer f.Close()
	if _, err = io.Copy(f, resp.Body); err != nil {
		return
	}
	outfile = f.Name()

	return
}

//youtube-dl --extract-audio --audio-format mp3 '[youtube url]'
func ripYoutubeMp3(url string) (outfile string, err error) {
	dir := tempdir()
	os.Chdir(dir)

	if err = command(youtubedl, "--extract-audio", "--audio-format", "mp3", url).Run(); err != nil {
		return
	}
	outfile = tempname(".mp3")
	filepath.Walk(dir, func(name string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			os.Rename(name, outfile)
			log.Println("outfile from youtube is", name)
		}
		return nil
	})

	return
}

//ffmpeg -ss [start second] -i [mp3 file] -t [length] -acodec copy [final file]
func cutMp3(infile, start, length string) (outfile string, err error) {
	os.Chdir(tmpdir)
	outfile = tempname(".mp3")

	err = command(ffmpeg, "-ss", start, "-i", infile, "-t", length, "-acodec", "copy", outfile).Run()
	return
}

//timidity [midi file] -Ow -o - | ffmpeg -i - -acodec libmp3lame -ab 64k [mp3 file]
func convertMidiToMp3(infile string) (outfile string, err error) {
	os.Chdir(tmpdir)
	outfile = tempname(".mp3")

	c1 := command(timidity, infile, "-Ow", "-o", "-")
	c2 := command(ffmpeg, "-i", "-", "-acodec", "libmp3lame", "-ab", "64k", outfile)

	r, w := io.Pipe()
	c1.Stdout = w
	c2.Stdin = r

	c1.Start()
	c2.Start()
	c1.Wait()
	w.Close()
	c2.Wait()

	return
}

//sox -m [first mp3] -v [volume adjustment] [second mp3] [final file]
func mixMp3(file1, file2, volumeAdjust string) (outfile string, err error) {
	os.Chdir(tmpdir)
	outfile = tempname(".mp3")
	err = command(sox, "-m", file1, "-v", volumeAdjust, file2, outfile).Run()
	return
}

//sox [infile] [outfile] speed [factor]
func changeSpeed(infile, speed string) (outfile string, err error) {
	os.Chdir(tmpdir)
	outfile = tempname(".mp3")
	err = command(sox, infile, outfile, "speed", speed).Run()
	return
}
