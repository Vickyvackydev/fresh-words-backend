Add-Type -AssemblyName System.Runtime.WindowsRuntime
[void][Windows.Security.Cryptography.CryptographicBuffer, Windows.Security.Cryptography, ContentType=WindowsRuntime]
[void][Windows.Storage.Streams.InMemoryRandomAccessStream, Windows.Storage.Streams, ContentType=WindowsRuntime]
[void][Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType=WindowsRuntime]
[void][Windows.Media.Ocr.OcrEngine, Windows.Media.Ocr, ContentType=WindowsRuntime]

# Render pages using our existing render_pdf.py with correct arguments
python render_pdf.py "C:\Users\USER\Downloads\trash-doc\Holiness Devontional_text_proof.pdf" "temp_ocr_pages"

$file = Get-Item 'temp_ocr_pages\page_0003.png'
$stream = [System.IO.File]::OpenRead($file.FullName)
$netStream = [System.IO.WindowsRuntimeStreamExtensions]::AsRandomAccessStream($stream)
$decoderOp = [Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($netStream)

# Custom awaiter
function Await-AsyncOperation ($asyncOp, $type) {
    while ($asyncOp.Status -eq [Windows.Foundation.AsyncStatus]::Started) {
        Start-Sleep -Milliseconds 10
    }
    return $asyncOp.GetResults()
}

$decoder = Await-AsyncOperation $decoderOp ([Windows.Graphics.Imaging.BitmapDecoder])
$bitmapOp = $decoder.GetSoftwareBitmapAsync()
$softwareBitmap = Await-AsyncOperation $bitmapOp ([Windows.Graphics.Imaging.SoftwareBitmap])

$ocrEngine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
$ocrOp = $ocrEngine.RecognizeAsync($softwareBitmap)
$ocrResult = Await-AsyncOperation $ocrOp ([Windows.Media.Ocr.OcrResult])

foreach ($line in $ocrResult.Lines) {
    if ($line.Words.Count -gt 0) {
        $rect = $line.Words[0].BoundingRect
        Write-Host ("Line: {0} | Y: {1} | H: {2}" -f $line.Text, $rect.Y, $rect.Height)
    }
}
