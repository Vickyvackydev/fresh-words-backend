$key = "AIzaSyDrgcLqypdDSPf8wMR4lR41yyB7ggy5nNg"
$endpoints = @(
    "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash:generateContent",
    "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash-lite:generateContent"
)

foreach ($ep in $endpoints) {
    Write-Host "Testing endpoint: $ep"
    $uri = "$ep`?key=$key"
    $body = @{
        contents = @(
            @{
                parts = @(
                    @{ text = "Hello" }
                )
            }
        )
    } | ConvertTo-Json -Depth 5

    try {
        $resp = Invoke-RestMethod -Uri $uri -Method Post -ContentType "application/json" -Body $body
        Write-Host "SUCCESS with $ep"
        Write-Host ($resp | ConvertTo-Json -Depth 3)
    } catch {
        Write-Host "FAILED with $ep : $($_.Exception.Message)"
        if ($_.Exception.Response) {
            $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
            Write-Host "Details: $($reader.ReadToEnd())"
        }
    }
}
