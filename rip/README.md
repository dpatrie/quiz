# Automated script to create a musical quiz

We read an input file. In the future we will read the file from Google Drive directly

### To process a "normal" question

- Rip mp3 from youtube
- Cut mp3 for question and answer with ffmpeg

### To process an "overlap" question

- Rip both mp3 from youtube
- Cut mp3 for question and answer with ffmpeg
- Mix them with sox (with optional volume adjustment)

### To process a "midi" question

- Download midi file
- Convert to mp3 by piping it to ffmpeg
- Cut mp3 for question and answer with ffmpeg

### To rip an mp3 from youtube
```
youtube-dl --extract-audio --audio-format mp3 '[youtube url]'
```
### To cut an mp3 with ffmpeg
```
ffmpeg -ss [start second] -i [mp3 file] -t [length] -acodec copy [final file]
```

### To convert a midi file to mp3
```
timidity [midi file] -Ow -o - | ffmpeg -i - -acodec libmp3lame -ab 64k [mp3 file]
```

### To mix to mp3 together
```
sox -m [first mp3] -v [optional volume adjustment] [second mp3] [final file]
```


