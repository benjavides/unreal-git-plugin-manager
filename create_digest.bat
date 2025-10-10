@echo off
call conda activate crowdtracker
gitingest . -o digest.txt -e dist/