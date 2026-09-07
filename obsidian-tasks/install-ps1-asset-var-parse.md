---
id: install-ps1-asset-var-parse
title: install.ps1 checksum-mismatch throw uses $asset: which PowerShell reads as a drive-qualified variable
status: todo
tags:
    - windows
    - install
verification_plan:
    - install.ps1 parses under Windows PowerShell 5.1 with no ParserError
    - a real install run on Windows reaches the checksum step and installs cozyphi.exe
created_at: "2026-09-07T12:32:07.000000Z"
updated_at: "2026-09-07T12:32:07.000000Z"
---

## Body

In scripts/install.ps1 the checksum-mismatch throw interpolates "$asset:" inside a double-quoted string. Windows PowerShell 5.1 reads $name: as a drive- or scope-qualified variable reference (the same trap the file already avoids on line 135 with ${sums}), so the string mis-parses instead of expanding $asset. Wrap the name as ${asset} to match the existing convention.

## Verification Plan

1. install.ps1 parses under Windows PowerShell 5.1 with no ParserError
2. a real install run on Windows reaches the checksum step and installs cozyphi.exe
