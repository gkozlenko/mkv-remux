package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	DefaultOrigLang = "eng"
	TargetLang      = "rus"
)

type ParsedMovie struct {
	Streams []ParsedStream
}

type ParsedStream struct {
	Index     int
	CodecName string `json:"codec_name"`
	CodecType string `json:"codec_type"`
	Channels  byte
	Tags      ParsedTag
}

type ParsedTag struct {
	Language string
}

func usage() {
	fmt.Println("MKV Remux v1.0")
	fmt.Println("Usage: mkv-remux [-lang=<video-lang>] <input-file> <output-file>")
}

func parse(source string) (ParsedMovie, error) {
	var movie ParsedMovie

	cmd := exec.Command("ffprobe", "-show_format", "-show_streams", "-print_format", "json", "-loglevel", "quiet", source)
	stdout, err := cmd.Output()
	if err != nil {
		return movie, err
	}

	err = json.Unmarshal(stdout, &movie)
	if err != nil {
		return movie, err
	}
	return movie, nil
}

func addAudioStream(output []string, stream ParsedStream, streamIndex byte) []string {
	output = append(output, "-map", fmt.Sprintf("0:%d", stream.Index))
	if stream.CodecName == "ac3" || stream.CodecName == "eac3" || stream.CodecName == "aac" {
		output = append(output, fmt.Sprintf("-c:%d", streamIndex), "copy")
	} else {
		output = append(output, fmt.Sprintf("-c:%d", streamIndex), "ac3", fmt.Sprintf("-b:%d", streamIndex), "640k")
	}
	output = append(output, fmt.Sprintf("-metadata:s:%d", streamIndex), fmt.Sprintf("language=%s", stream.Tags.Language))
	return output
}

func addSubtitleStream(output []string, stream ParsedStream, streamIndex byte) []string {
	output = append(
		output,
		"-map", fmt.Sprintf("0:%d", stream.Index),
		fmt.Sprintf("-c:%d", streamIndex), "copy",
		fmt.Sprintf("-metadata:s:%d", streamIndex), fmt.Sprintf("language=%s", stream.Tags.Language),
	)
	return output
}

func mux(source string, target string, videoLang string) (string, error) {
	movie, err := parse(source)
	if err != nil {
		return "", err
	}

	output := []string{
		"ffmpeg",
		// generate missing timestamps
		"-fflags", "+genpts",
		"-i", fmt.Sprintf("\"%s\"", source),
		// remove metadata
		"-map_chapters", "-1",
		"-map_metadata", "-1",
		// disable default subtitles
		"-default_mode", "infer_no_subs",
	}

	// index of current stream
	var streamIndex byte = 0
	var origLang = videoLang

	// get video stream
	for _, stream := range movie.Streams {
		if stream.CodecType == "video" {
			if origLang == "" && stream.Tags.Language != "" {
				origLang = stream.Tags.Language
			}
			if origLang == "" {
				fmt.Fprintln(os.Stderr, "[warn] video language is not defined, use default one:", DefaultOrigLang)
				origLang = DefaultOrigLang
			}
			output = append(
				output,
				"-map", fmt.Sprintf("0:%d", stream.Index),
				fmt.Sprintf("-c:%d", streamIndex), "copy",
				fmt.Sprintf("-metadata:s:%d", streamIndex), fmt.Sprintf("language=%s", origLang),
			)
			streamIndex++
			break
		}
	}

	// list of languages
	languages := []string{TargetLang}
	if origLang != TargetLang {
		languages = append(languages, origLang)
	}
	if origLang != DefaultOrigLang {
		languages = append(languages, DefaultOrigLang)
	}

	// get audio stream
	for _, lang := range languages {
		for _, stream := range movie.Streams {
			if stream.CodecType == "audio" && stream.Tags.Language == lang && stream.Channels <= 6 {
				output = addAudioStream(output, stream, streamIndex)
				streamIndex++
				break
			}
		}
	}

	// get subtitles
	for _, lang := range languages {
		for _, stream := range movie.Streams {
			if stream.CodecType == "subtitle" && stream.Tags.Language == lang {
				output = addSubtitleStream(output, stream, streamIndex)
				streamIndex++
				break
			}
		}
	}

	// output
	output = append(output, fmt.Sprintf("\"%s\"", target))

	return strings.Join(output[:], " "), nil
}

func main() {
	videoLang := flag.String("lang", "", "video language")
	flag.Parse()
	var args = flag.Args()

	if len(args) != 2 {
		usage()
	} else {
		cmd, err := mux(args[0], args[1], *videoLang)
		if err != nil {
			print("Unable to mux file")
		} else {
			print(cmd)
		}
	}
}
