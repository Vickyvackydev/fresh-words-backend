Add-Type -AssemblyName System.Runtime.WindowsRuntime
[void][Windows.Security.Cryptography.CryptographicBuffer, Windows.Security.Cryptography, ContentType=WindowsRuntime]
[void][Windows.Storage.Streams.InMemoryRandomAccessStream, Windows.Storage.Streams, ContentType=WindowsRuntime]
[void][Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType=WindowsRuntime]
[void][Windows.Media.Ocr.OcrEngine, Windows.Media.Ocr, ContentType=WindowsRuntime]

# Render page 3
python render_pdf.py "C:\Users\USER\Downloads\trash-doc\Holiness Devontional_text_proof.pdf" "temp_ocr_pages"

$file = Get-Item 'temp_ocr_pages\page_0003.png'
$stream = [System.IO.File]::OpenRead($file.FullName)
$netStream = [System.IO.WindowsRuntimeStreamExtensions]::AsRandomAccessStream($stream)
$decoderOp = [Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($netStream)

# Reflection based Awaiter (safe for WinRT async in PowerShell)
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

$decoder = Await-AsyncOperation $decoderOp ([Windows.Graphics.Imaging.BitmapDecoder])
$bitmapOp = $decoder.GetSoftwareBitmapAsync()
$softwareBitmap = Await-AsyncOperation $bitmapOp ([Windows.Graphics.Imaging.SoftwareBitmap])

$ocrEngine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
$ocrOp = $ocrEngine.RecognizeAsync($softwareBitmap)
$ocrResult = Await-AsyncOperation $ocrOp ([Windows.Media.Ocr.OcrResult])

$prevY = -1.0
$prevH = -1.0
foreach ($line in $ocrResult.Lines) {
    if ($line.Words.Count -gt 0) {
        $firstWord = $line.Words | Select-Object -First 1
        $rect = $firstWord.BoundingRect
        $currentY = [double]$rect.Y
        $currentH = [double]$rect.Height
        
        if ($prevY -ne -1.0) {
            $verticalGap = $currentY - ($prevY + $prevH)
            $ratio = $verticalGap / $prevH
            Write-Host ("Line: '{0}' | Y: {1} | H: {2} | Gap: {3} | Gap/H Ratio: {4:N2}" -f $line.Text, $currentY, $currentH, $verticalGap, $ratio)
        } else {
            Write-Host ("Line: '{0}' | Y: {1} | H: {2}" -f $line.Text, $currentY, $currentH)
        }
        
        $prevY = $currentY
        $prevH = $currentH
    }
}
