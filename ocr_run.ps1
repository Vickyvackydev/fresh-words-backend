param(
    [string]$PdfPath = "C:\Users\USER\Downloads\trash-doc\Holiness Devontional_text_proof.pdf",
    [string]$OutputFile = "C:\Users\USER\Downloads\Holiness_Devotional_Extracted.txt"
)

# Enable UTF-8 encoding for Output
$OutputEncoding = [System.Text.Encoding]::UTF8

Write-Host "Starting OCR conversion tool..."
Write-Host "PDF Path: $PdfPath"
Write-Host "Output File: $OutputFile"

# 1. Setup temporary directory
$tempDir = Join-Path $PSScriptRoot "temp_ocr_pages"
if (Test-Path $tempDir) {
    Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
}
New-Item -ItemType Directory -Path $tempDir | Out-Null

# 2. Render PDF to PNGs using Python + PyMuPDF
Write-Host "Rendering PDF pages to PNG (this may take a few moments)..."
python render_pdf.py "$PdfPath" "$tempDir"

if (-not (Test-Path $tempDir) -or (Get-ChildItem $tempDir).Count -eq 0) {
    Write-Error "Failed to render PDF pages. Make sure Python is installed and 'pip install pymupdf' has completed."
    exit 1
}

# 3. Initialize Windows OCR
Write-Host "Loading Windows OCR assemblies..."
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[void][Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType = WindowsRuntime]
[void][Windows.Graphics.Imaging.SoftwareBitmap, Windows.Graphics.Imaging, ContentType = WindowsRuntime]
[void][Windows.Storage.Streams.RandomAccessStream, Windows.Storage, ContentType = WindowsRuntime]

$ocrEngine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
if ($ocrEngine -eq $null) {
    Write-Error "Windows OCR Engine could not be initialized. Ensure English language pack OCR features are installed."
    exit 1
}

# 3.5 Setup Async helper
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$asTaskGeneric = ([System.WindowsRuntimeSystemExtensions].GetMethods() | 
    Where-Object { 
        $_.Name -eq 'AsTask' -and 
        $_.GetParameters().Count -eq 1 -and 
        $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1' 
    })[0]

function Await-AsyncOperation($asyncOp, $resultType) {
    $asTask = $script:asTaskGeneric.MakeGenericMethod($resultType)
    $netTask = $asTask.Invoke($null, @($asyncOp))
    $netTask.Wait()
    return $netTask.Result
}

# 4. Process each page image
$pngFiles = Get-ChildItem -Path $tempDir -Filter "*.png" | Sort-Object Name
$total = $pngFiles.Count
Write-Host "Found $total pages to process."

# Clear existing output file
if (Test-Path $OutputFile) {
    Remove-Item -Path $OutputFile -Force
}
New-Item -ItemType File -Path $OutputFile | Out-Null

$pageIndex = 1
foreach ($file in $pngFiles) {
    Write-Host "Running OCR on page $pageIndex of $total ($($file.Name))..."
    
    # Load image and stream to WinRT OcrEngine
    $bitmap = [System.Drawing.Bitmap]::FromFile($file.FullName)
    $memoryStream = New-Object System.IO.MemoryStream
    $bitmap.Save($memoryStream, [System.Drawing.Imaging.ImageFormat]::Png)
    $bitmap.Dispose() # release file handle immediately
    
    $memoryStream.Position = 0
    Add-Type -AssemblyName System.Runtime.WindowsRuntime
    $randomAccessStream = [System.IO.WindowsRuntimeStreamExtensions]::AsRandomAccessStream($memoryStream)
    
    $decoderOp = [Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($randomAccessStream)
    $decoder = Await-AsyncOperation $decoderOp ([Windows.Graphics.Imaging.BitmapDecoder])
    
    $bitmapOp = $decoder.GetSoftwareBitmapAsync()
    $softwareBitmap = Await-AsyncOperation $bitmapOp ([Windows.Graphics.Imaging.SoftwareBitmap])
    
    $ocrOp = $ocrEngine.RecognizeAsync($softwareBitmap)
    $ocrResult = Await-AsyncOperation $ocrOp ([Windows.Media.Ocr.OcrResult])
    
    $lines = @()
    $prevY = -1.0
    $prevH = -1.0
    foreach ($ocrLine in $ocrResult.Lines) {
        if ($ocrLine.Words.Count -gt 0) {
            $firstWord = $ocrLine.Words | Select-Object -First 1
            $rect = $firstWord.BoundingRect
            $currentY = [double]$rect.Y
            $currentH = [double]$rect.Height
            
            if ($prevY -ne -1.0) {
                $verticalGap = $currentY - ($prevY + $prevH)
                # Spacing between paragraphs is typically much larger (around 90% or more of line height)
                $threshold = $prevH * 0.9
                if ($verticalGap -gt $threshold) {
                    $lines += ""
                }
            }
            
            $prevY = $currentY
            $prevH = $currentH
        }
        $lines += $ocrLine.Text
    }
    $text = $lines -join "`r`n"
    
    # Write page content to output file
    Add-Content -Path $OutputFile -Value "`n`n$text"
    
    $pageIndex++
}

# 5. Cleanup
Write-Host "Cleaning up temp files..."
Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue

# 6. Convert Text to Formatted DOCX
$DocxOutputFile = $OutputFile.Replace(".txt", ".docx")
Write-Host "Converting text to formatted Word document: $DocxOutputFile..."
python txt_to_docx.py "$OutputFile" "$DocxOutputFile"

Write-Host "OCR conversion finished successfully!"
Write-Host "Plain Text Output: $OutputFile"
Write-Host "Formatted DOCX Output: $DocxOutputFile"
