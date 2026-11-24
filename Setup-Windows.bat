@echo off
setlocal
cd /d "%~dp0"
where ffmpeg >nul 2>nul || goto :ffmpeg
where ffprobe >nul 2>nul || goto :ffmpeg
if exist ".venv\Scripts\python.exe" goto :install
where py >nul 2>nul
if errorlevel 1 goto :python
py -3.11 -m venv .venv
if errorlevel 1 py -3.12 -m venv .venv
if errorlevel 1 goto :python
:install
".venv\Scripts\python.exe" -m pip install -r requirements.txt
if errorlevel 1 goto :failed
echo Preparing the local Whisper model. Internet is needed for this download.
".venv\Scripts\python.exe" -m vodcut model
if errorlevel 1 goto :model
echo Setup complete. Open Start-VODCut.bat to review a VOD.
pause
exit /b 0
:ffmpeg
echo Install FFmpeg with ffprobe on PATH. See README.md, then run setup again.
goto :failed
:python
echo Install 64-bit Python 3.11 or 3.12 with the Windows py launcher. See README.md.
goto :failed
:model
echo Model download failed. Retry Setup-Windows.bat or run the app with transcription disabled.
:failed
pause
exit /b 1
