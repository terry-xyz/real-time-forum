.PHONY: run

ifeq ($(OS),Windows_NT)
SHELL := powershell.exe
.SHELLFLAGS := -NoProfile -Command

windows_run = $$exe = Join-Path ([IO.Path]::GetTempPath()) ('forum-' + [guid]::NewGuid() + '.exe'); \
		try { \
			go build -o $$exe .; \
			$$buildExit = $$LASTEXITCODE; \
			$$fallback = 'C:\w64devkit\bin\gcc.exe'; \
			if ($$buildExit -ne 0 -and (Test-Path -LiteralPath $$fallback -PathType Leaf) -and $$env:CC -ne $$fallback) { \
				Write-Host 'Build failed. Retrying with w64devkit GCC.'; \
				$$env:CC = $$fallback; \
				go build -o $$exe .; \
				$$buildExit = $$LASTEXITCODE; \
			}; \
			if ($$buildExit -ne 0) { exit $$buildExit }; \
			& $$exe; \
			exit $$LASTEXITCODE; \
		} finally { if (Test-Path -LiteralPath $$exe) { Remove-Item -LiteralPath $$exe } }

run:
	@$(windows_run)
else
run:
	go run .
endif
