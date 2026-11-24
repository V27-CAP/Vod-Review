@echo off
setlocal
set "app=%~dp0downloads\VODCut-Studio.exe"
if exist "%app%" for %%F in ("%app%") do if %%~zF GTR 1000000 (
  start "" "%app%"
  exit /b 0
)
echo Baixe VODCut-Studio.exe na pasta downloads do repositorio e abra o arquivo.
pause
exit /b 1
