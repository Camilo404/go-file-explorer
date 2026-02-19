param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [switch]$PauseAtEnd
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message)
    Write-Host "[SMOKE] $Message" -ForegroundColor Cyan
}

function Invoke-Api {
    param(
        [string]$Method,
        [string]$Url,
        [hashtable]$Headers,
        [object]$Body
    )

    $params = @{
        Method = $Method
        Uri    = $Url
    }

    if ($Headers) {
        $params.Headers = $Headers
    }

    if ($null -ne $Body) {
        $params.ContentType = "application/json"
        $params.Body = ($Body | ConvertTo-Json -Depth 8)
    }

    return Invoke-RestMethod @params
}

function Upload-FileMultipart {
    param(
        [string]$Url,
        [string]$Token,
        [string]$DestinationPath,
        [string]$FilePath
    )

    Add-Type -AssemblyName System.Net.Http

    $handler = New-Object System.Net.Http.HttpClientHandler
    $client = New-Object System.Net.Http.HttpClient($handler)

    try {
        $client.DefaultRequestHeaders.Authorization = New-Object System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", $Token)

        $form = New-Object System.Net.Http.MultipartFormDataContent

        $pathContent = New-Object System.Net.Http.StringContent($DestinationPath)
        $form.Add($pathContent, "path")

        $fileBytes = [System.IO.File]::ReadAllBytes($FilePath)
        $byteContent = [System.Net.Http.ByteArrayContent]::new($fileBytes)
        $byteContent.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse("application/octet-stream")
        $form.Add($byteContent, "files", [System.IO.Path]::GetFileName($FilePath))

        $response = $client.PostAsync($Url, $form).GetAwaiter().GetResult()
        $raw = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()

        if (-not $response.IsSuccessStatusCode) {
            throw "Upload fallo: HTTP $($response.StatusCode) - $raw"
        }

        return ($raw | ConvertFrom-Json)
    }
    finally {
        $client.Dispose()
    }
}

function Test-ApiHealth {
    param([string]$Url)

    try {
        $resp = Invoke-WebRequest -Method GET -Uri "$Url/health" -UseBasicParsing -TimeoutSec 5
        return ($resp.StatusCode -eq 200)
    }
    catch {
        return $false
    }
}

$testDirName = "smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
$testDirPath = "/$testDirName"
$testFilePath = Join-Path $env:TEMP ("smoke-" + [guid]::NewGuid().ToString("N") + ".txt")
$token = $null

try {
    Write-Step "Base URL: $BaseUrl"

    if (-not (Test-ApiHealth -Url $BaseUrl)) {
        throw "La API no responde en $BaseUrl. Inicia el servidor con: go run ./cmd/server"
    }

    Write-Step "1) Login"
    $loginBody = @{
        username = $Username
        password = $Password
    }
    $loginResp = Invoke-Api -Method "POST" -Url "$BaseUrl/api/v1/auth/login" -Body $loginBody
    $token = $loginResp.data.access_token

    if ([string]::IsNullOrWhiteSpace($token)) {
        throw "No se pudo obtener access_token"
    }

    $authHeaders = @{ Authorization = "Bearer $token" }

    Write-Step "2) Crear carpeta de prueba: $testDirPath"
    $createBody = @{
        path = "/"
        name = $testDirName
    }
    $null = Invoke-Api -Method "POST" -Url "$BaseUrl/api/v1/directories" -Headers $authHeaders -Body $createBody

    Write-Step "3) Crear archivo temporal"
    "archivo de prueba smoke $(Get-Date -Format o)" | Out-File -FilePath $testFilePath -Encoding utf8

    Write-Step "4) Subir archivo"
    $uploadResp = Upload-FileMultipart -Url "$BaseUrl/api/v1/files/upload" -Token $token -DestinationPath $testDirPath -FilePath $testFilePath
    if (-not $uploadResp.success) {
        throw "Upload respondió success=false"
    }

    Write-Step "5) Listar carpeta"
    $listResp = Invoke-Api -Method "GET" -Url "$BaseUrl/api/v1/files?path=$([uri]::EscapeDataString($testDirPath))&page=1&limit=50" -Headers $authHeaders
    if (-not $listResp.success) {
        throw "List respondió success=false"
    }

    Write-Step "6) Buscar archivo"
    $searchQuery = "smoke"
    $searchResp = Invoke-Api -Method "GET" -Url "$BaseUrl/api/v1/search?q=$searchQuery&path=$([uri]::EscapeDataString($testDirPath))&type=file&page=1&limit=20" -Headers $authHeaders
    if (-not $searchResp.success) {
        throw "Search respondió success=false"
    }

    $itemsCount = 0
    if ($searchResp.data -and $searchResp.data.items) {
        $itemsCount = @($searchResp.data.items).Count
    }

    Write-Step "7) Resultado"
    Write-Host "Smoke test OK" -ForegroundColor Green
    Write-Host "- Carpeta: $testDirPath"
    Write-Host "- Archivo temporal: $testFilePath"
    Write-Host "- Resultados de búsqueda: $itemsCount"
    $global:LASTEXITCODE = 0
}
catch {
    Write-Host ("Smoke test FALLO: " + $_.Exception.Message) -ForegroundColor Red
    $global:LASTEXITCODE = 1
}
finally {
    if (Test-Path $testFilePath) {
        Remove-Item $testFilePath -Force -ErrorAction SilentlyContinue
    }

    if (-not [string]::IsNullOrWhiteSpace($token)) {
        try {
            $headers = @{ Authorization = "Bearer $token" }
            $deleteBody = @{ paths = @($testDirPath) }
            $null = Invoke-Api -Method "DELETE" -Url "$BaseUrl/api/v1/files" -Headers $headers -Body $deleteBody
            Write-Step "Limpieza completada: $testDirPath"
        }
        catch {
            Write-Host ("No se pudo limpiar carpeta de prueba: " + $_.Exception.Message) -ForegroundColor Yellow
        }
    }

    if ($PauseAtEnd) {
        Write-Host "`nPresiona Enter para cerrar..." -ForegroundColor DarkGray
        [void](Read-Host)
    }
}
