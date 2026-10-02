$key = "AIzaSyDrgcLqypdDSPf8wMR4lR41yyB7ggy5nNg"
$url = "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash-lite:generateContent?key=$key"
$body = @{
    contents = @(
        @{
            parts = @(
                @{
                    text = 'Translate the following devotional JSON fields into French: {"title": "Walking in Faith", "body": "Trust in the Lord with all your heart."}. Return JSON strictly matching: {"title": string, "body": string}'
                }
            )
        }
    )
    generationConfig = @{
        responseMimeType = "application/json"
    }
} | ConvertTo-Json -Depth 5

try {
    $resp = Invoke-RestMethod -Uri $url -Method Post -ContentType "application/json" -Body $body
    Write-Host "Success: $($resp.candidates[0].content.parts[0].text)"
} catch {
    Write-Host "Status: $($_.Exception.Response.StatusCode)"
    $stream = $_.Exception.Response.GetResponseStream()
    $reader = New-Object System.IO.StreamReader($stream)
    Write-Host "Error Body: $($reader.ReadToEnd())"
}
