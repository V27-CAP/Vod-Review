# VODCut Studio

A local review desk for editors clipping long livestreams. Load a VOD, rank moments, play the proposed ranges, trim and approve them, then open the selects sequence in Adobe Premiere Pro. The native Windows application supports Brazilian Portuguese and English.

## Baixar para Windows

Abra [VODCut-Studio.exe](https://github.com/V27-CAP/Vod-Review/blob/main/downloads/VODCut-Studio.exe) e clique em **Download** no GitHub. Entre com a conta que tem acesso a este repositório privado.

1. Feche a versão anterior do VODCut e dê dois cliques no novo **VODCut-Studio.exe**.
2. Clique em **Escolher vídeo…**. Use **Procurar vídeo…** ou cole o caminho da gravação e clique em **Analisar vídeo**. Você também pode arrastar um vídeo para a janela.
3. Selecione um destaque para ver a prévia, ajustar o nome e os tempos e aprovar o corte. **Exportar aprovados…** gera XML para o Premiere, MP4, CSV ou JSON. O menu **Aparência** oferece vermelho, escuro e branco.

A janela usa controles nativos do Windows e abre diretamente pelo executável. Python, FFmpeg e Whisper só entram em ação quando você solicita uma operação. O pacote também inclui um reprodutor de prévia para computadores sem o codec do Windows. A primeira análise prepara as ferramentas locais e reutiliza a instalação da versão anterior quando disponível. Reserve 1,4 GB livres para as ferramentas, além do espaço para seus vídeos e cortes. [Ver a interface do aplicativo](downloads/VODCut-Interface.png)

### Rebuild the native Windows executable

With Git LFS installed, run `git lfs pull` to fetch the packaged offline runtime. With Go 1.23+ and Python 3.11+ on PATH, run `python launcher/package.py downloads/VODCut-Studio.exe build/VODCut-Studio.exe`. This embeds the native desktop code and worker with the exact runtime ZIP, so the Windows app recognizes earlier installations. The Common Controls manifest is in `launcher/studio.manifest`. Regenerate its Go resource after changing it with `rsrc -arch amd64 -manifest studio.manifest -o resource_windows_amd64.syso` in the launcher directory.

## Optional Python CLI and browser interface from source

1. Install **64-bit Python 3.11 or 3.12** from python.org, including the Windows Python launcher and Tcl/Tk.
2. Install a **full FFmpeg build** with FFprobe, H.264 encoding (libx264) and AAC. With Windows Package Manager, run `winget install --id Gyan.FFmpeg -e`. Alternatively, unpack a Windows build linked from ffmpeg.org/download.html and add its `bin` directory to your user PATH. Open a new Command Prompt and check both `ffmpeg -version` and `ffprobe -version`.
3. Extract the entire ZIP to a writable folder, for example `D:\Tools\VODCut`. Double-click **Setup-Windows.bat** once. It creates a private Python environment, installs the packages and downloads the multilingual Whisper base model. Initial setup needs internet access. To use a different model, edit `config.toml` before setup.
4. To explicitly start the optional browser interface for development, run `.venv\Scripts\python -m vodcut serve`. The desktop EXE uses direct process communication and starts without an HTTP service.
5. Choose **Carregar VOD / Load VOD**. Browse to an MP4, MKV or MOV, or paste its full path. Add a chat file if available. Select signals and duration limits, then start analysis. Choose **EN** or **PT** in the header to switch languages.

Analysis uses local files and CPU INT8 inference, including on a machine with an AMD RX 570. The local server binds to loopback. The browser uses bundled assets and accesses the video through the local backend. Disk space and processing time are the practical constraints. A four-hour recording uses about 461 MB of PCM cache and an eight-hour recording uses about 922 MB, plus previews, transcripts, model weights and any requested exports.

### Manual setup

Run these commands from the extracted directory in Command Prompt. On Linux/macOS, use a Python 3.11+ virtual environment and its corresponding Python executable. Linux native file dialogs need the system Tk package. A full path can also be entered directly.

```bat
py -3.11 -m venv .venv
.venv\Scripts\python -m pip install -r requirements.txt
.venv\Scripts\python -m vodcut model
.venv\Scripts\python -m vodcut serve
```

For an offline workstation, install the dependencies from an appropriate wheelhouse and set `analysis.model` to a copied CTranslate2 faster-whisper model directory containing its tokenizer. Use a TOML literal path such as `model = 'D:\Models\faster-whisper-base'`. The detector requests local model files only. You can analyze audio, chat and video without model weights by deselecting transcription. An internet connection is only needed to acquire the software and weights.

## Reviewing a VOD

The queue is sorted by score. Select a card to inspect its peak, normalized signal contributions, transcript and a preview of the exact selected source interval. The first playback generates a short H.264/AAC proxy, including for MKV or MOV sources. The video player's time starts at the beginning of that excerpt. The monitor readout and inspector show original VOD time. The signal strip summarizes the whole VOD and highlights the selected interval.

Edit the name and source-relative start/end in seconds. Use **Save changes**, **Approve** or **Reject** to commit the fields. The peak must remain inside the selection so the exported marker stays meaningful. A decision can be changed from any queue filter. Approved clips alone enter the export. New analyses with identical settings restore saved decisions. Changing settings starts another review for that recording, preserving the earlier review on disk.

**Stop** interrupts at the next processing checkpoint. An in-flight Whisper chunk, FFmpeg call or chat parse can finish before the job stops. To resume, use the same original file, chat and settings and start analysis again. The last completed review reopens on startup.

## Premiere Pro

1. Approve at least one clip and select **Export clips**. Choose **Premiere Pro · XML**. Leave **Match source** selected, or choose **1080 × 1920** for a portrait editing sequence. You can request CSV, JSON and individual MP4s in the same export.
2. Open the export folder from the completion message. Each batch gets a unique folder under `%USERPROFILE%\VODCut-data\exports`.
3. In Premiere Pro choose **File > Import** (Ctrl+I), select **sequence.xml**, then open the imported sequence in the Project panel. The sequence assembles approved clips in source time order. Each video clip carries its reviewed name and links to the original VOD. Sequence markers identify reaction peaks. The source's first audio stream is linked channel by channel.
4. If Premiere asks to locate media, choose the original file at its current location. Scrub to the first and last markers, compare the spoken words to the app preview, and check audio sync. Finish reframing and captioning in Premiere. Portrait changes the XML sequence dimensions. The optional rendered MP4s preserve the original framing.

The exporter writes Final Cut Pro 7 XML, xmeml version 4. It reopens the XML and validates source references, linked item IDs, source and sequence frame ranges, contiguous tracks and marker placement. Native Premiere import requires a check on your installed Premiere version. Include the Premiere version, exact import message or translation report, source container, frame rate, audio channels and a copy of the XML if you report an issue. File paths in the XML may be edited for privacy before sharing.

**Timing and media assumptions.** Standard progressive, square-pixel, constant-frame-rate livestream recordings are the primary target. Common fractional NTSC rates use their integer timebase and NTSC flag. XML boundaries round to the nearest frame. For VFR or unusual rates, the XML uses the nearest representable integer/NTSC rate derived from the average. If sync drifts on a long recording, convert it to constant frame rate and analyze that file as the new source. Premiere's ability to open MKV depends on its version and codecs. If Premiere cannot read a source, remux or transcode it to a supported MP4/MOV and load that file in VODCut before exporting. For multitrack recordings, prepare a program mix as the first audio stream.

## Signals and tuning

The pipeline uses one-second score bins. It computes robust percentile normalization per recording, combines enabled signals by relative weight, and adds a context bonus where speech surrounds a peak. It ranks local maxima and applies non-maximum suppression to the proposed intervals. It keeps up to the configured limit. A quiet recording may yield zero proposals.

| Setting in config.toml | Effect |
| --- | --- |
| analysis.model, language, cpu_threads, beam_size | Multilingual base and automatic language detection per chunk by default. Set language to pt, en or ja for a known language. Tiny is faster, small needs more CPU and memory. Run the model command after changing to a new model name. |
| analysis.chunk_seconds | Default 180. Shorter blocks reduce retry cost. Words near a hard block boundary may split. |
| weights | Relative weights of loudness above local baseline, energy change, vocal bursts, reaction wording, chat rate, chat laughter, unique participants and visual motion. A zero weight excludes a signal from scoring. Disable its module to skip processing. |
| features | Configure the loudness baseline, silence threshold, reaction and laughter patterns, vocal pulse sensitivity, scene sampling, chat bins and chat delay. Increase chat_delay_seconds when viewers respond late. |
| clips | Default 15–90 s, 14 s of setup and 9 s of reaction after the peak bin. Adjust rolls and snapping radius for more context. Sentence boundaries and silence transitions can move the cut. A VOD shorter than the minimum uses the available duration. |
| clips.min_score, top_n, context_bonus | Control the admission threshold, proposal count and preference for peaks surrounded by speech. The score ranks suggestions within a recording. |
| judge | Configure a local model server, top shortlist size and score blend. This stage is off by default. |

Laughter and screams use acoustic heuristics based on librosa RMS, zero crossings, changes in energy and repeated pulses. Game sound, music and multiple voices can trigger the same patterns. Transcript reaction words, chat reactions and spoken setup provide additional evidence. Quiet jokes without these cues can be missed. The optional visual signal measures frame-to-frame differences in small grayscale samples. It is intended for broad scene or motion changes.

### Chat formats

Use a UTF-8 CSV with columns such as `time,user,message`, a JSON array from chat-downloader, or an object containing a `comments`, `messages` or `chat` array. TwitchDownloader-style `content_offset_seconds`, `commenter.id` and `message.body` are supported. Chat-downloader `time_in_seconds`, `author.name` and `message` work for Twitch or YouTube. Time fields accept seconds or HH:MM:SS with optional fractions. They must be relative to the start of this VOD. Convert absolute wall-clock times before importing. The streaming parser reads records incrementally. Add a field adapter in `vodcut/chat.py` for another exporter.

### Optional local LLM

Start an OpenAI-compatible model server on a loopback IP such as `127.0.0.1`. Load a model that can follow JSON instructions. Set `judge.endpoint` and `judge.model` to its actual chat completions URL and model ID. If it requires a token, set the environment variable named by `judge.key_env` before opening the app. Enable the local LLM checkbox for that run. It receives only the transcript of each shortlisted clip and rates humor, surprise and understandability, with a short reason in each UI language. Understandability has double weight by default. A failed judge request reports an error. Disable it to rerun using cached analysis. A different provider can implement the small `Judge` protocol in `vodcut/judge.py`.

## Evaluate against your own selects

Start with the **unedited source video** and its manual highlights. Use source-relative timestamps. A reference can be a single instant, such as the punchline, or a start/end range. CSV accepts `start,end` with an optional end. JSON accepts an array such as `[42.5, {"start": "00:12:10", "end": "00:12:26"}]`. Example input shapes are in `examples/`.

```bat
.venv\Scripts\python -m vodcut.evaluate "D:\VODs\stream.mp4" "D:\VODs\my-selects.csv" --chat "D:\VODs\chat.json" --k 5 10 20
```

The evaluator runs the detector and ignores saved review edits. For each K, it matches the top K candidates to manual moments one to one using maximum bipartite matching. An interval hit requires positive temporal overlap. Point hits satisfy start ≤ point < end. Recall@K divides distinct hits by the number of manual references. Precision@K divides hits by K, including when fewer than K proposals qualify. Output also includes the returned count.

For example, if the top 5 match 3 of your 8 manual selections, precision@5 is 0.60 and recall@5 is 0.375. If recall is low, inspect missing moments, adjust one weight or threshold, and rerun with `--config my-settings.toml`. Compare the same K on additional VODs you did not use for tuning. A very wide manual interval makes overlap easier and can inflate results.

### CLI analysis and tests

```bat
.venv\Scripts\python -m vodcut detect "D:\VODs\stream.mkv" --chat "D:\VODs\chat.csv"
.venv\Scripts\python -m vodcut --config my-settings.toml detect "D:\VODs\stream.mov" --no-transcription
.venv\Scripts\python -m unittest discover -s tests -v
```

CLI detection prints candidates as JSON on stdout and progress on stderr. Thumbnails are cached in the source-specific `thumbs` folder. Evaluation also accepts `--no-transcription`.

## Storage and implementation

`VODCUT_CACHE` changes the root for project caches, saved reviews and exports. Set it to a drive with enough space before starting VODCut. The default is `VODCut-data` in your Windows profile. Whisper model weights use the Hugging Face cache or the configured local model directory. Keep exports and review files you need before clearing a project cache.

| Module | Responsibility |
| --- | --- |
| vodcut/media.py | FFprobe source metadata, FFmpeg PCM blocks, local multilingual word timestamps |
| vodcut/signals.py, detector.py | Cached acoustic and optional visual features, normalized ranking, natural boundaries and thumbnails |
| launcher/ | Native Win32 desktop, on-demand processing worker and portable EXE packager |
| vodcut/server.py, web/ | Loopback FastAPI service, persistent review and bilingual browser interface |
| vodcut/export.py | Validated xmeml v4, CSV, JSON and source-framed MP4 rendering |
| vodcut/chat.py, judge.py, evaluate.py | Streaming chat adapters, optional local judging and evaluation |
| tests/ | Scoring, bounds, XML integrity, chat formats, evaluation and judge cache tests |

The source cache identity uses the resolved path, byte size and modification time. Analysis stages additionally key their output by the parameters they use. Finished chunks are committed atomically. Retuning ranking weights can reuse decoded audio, transcript and features. Cache and reviews are local to the workstation and original file identity.

## Milestone assumptions and checks

1. **Media and transcription.** Used CPU INT8, one primary audio stream and bounded PCM blocks. Verified real FFmpeg extraction, word timestamps, automatic English detection and cache reuse with synthesized speech.
2. **Detection.** Used acoustic proxies for laughs and shouts and optional sampled motion. Tested a planted reaction, suppression of nearby peaks, timing bounds and a flat signal. A four-hour synthetic MKV with one spoken event near the end also ran through offline base-model transcription and ranking.
3. **Review.** Selected a local browser UI and on-demand proxies. Exercised import, playback metadata, language switch, rename, trimming, rejection, approval and persistence in Chromium.
4. **Export.** Approved clips form one chronological sequence with markers in sequence time. Validated the generated XML and checked actual CSV, JSON and MP4 output. Premiere import and Windows shell launch require confirmation on the target workstation.
5. **Evaluation.** Selected local-only judging and one-to-one matching. Tested chat adapters, cache behavior, overlap conventions and the evaluation CLI. The synthetic checks establish timing and plumbing. Ranking quality for a particular editor requires their labeled VODs.
