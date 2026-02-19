param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$AdminUsername = "admin",
    [string]$AdminPassword = "admin123",
    [switch]$PauseAtEnd
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message)
    Write-Host "[E2E] $Message" -ForegroundColor Cyan
}

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Invoke-JsonApi {
    param(
        [string]$Method,
        [string]$Url,
        [hashtable]$Headers,
        [object]$Body,
        [int]$ExpectedStatus = 200
    )

    $params = @{
        Method      = $Method
        Uri         = $Url
        ErrorAction = "Stop"
    }

    if ($Headers) {
        $params.Headers = $Headers
    }

    if ($null -ne $Body) {
        $params.ContentType = "application/json"
        $params.Body = ($Body | ConvertTo-Json -Depth 20)
    }

    $resp = Invoke-WebRequest @params

    if ($resp.StatusCode -ne $ExpectedStatus) {
        throw "HTTP inesperado en $Method ${Url}: esperado $ExpectedStatus, recibido $($resp.StatusCode). Body: $($resp.Content)"
    }

    $json = $null
    if (-not [string]::IsNullOrWhiteSpace($resp.Content)) {
        $json = $resp.Content | ConvertFrom-Json
    }

    return [pscustomobject]@{
        StatusCode = [int]$resp.StatusCode
        Body       = $json
        Raw        = $resp
    }
}

function Invoke-MultipartUpload {
    param(
        [string]$Url,
        [string]$Token,
        [string]$DestinationPath,
        [string]$FilePath,
        [int]$ExpectedStatus = 200
    )

    Add-Type -AssemblyName System.Net.Http

    $client = [System.Net.Http.HttpClient]::new()
    try {
        $client.DefaultRequestHeaders.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new("Bearer", $Token)

        $form = [System.Net.Http.MultipartFormDataContent]::new()
        $form.Add([System.Net.Http.StringContent]::new($DestinationPath), "path")

        $fileBytes = [System.IO.File]::ReadAllBytes($FilePath)
        $fileContent = [System.Net.Http.ByteArrayContent]::new($fileBytes)
        $fileContent.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse("application/octet-stream")
        $form.Add($fileContent, "files", [System.IO.Path]::GetFileName($FilePath))

        $response = $client.PostAsync($Url, $form).GetAwaiter().GetResult()
        $raw = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()

        if ([int]$response.StatusCode -ne $ExpectedStatus) {
            throw "Upload falló: esperado $ExpectedStatus, recibido $([int]$response.StatusCode). Body: $raw"
        }

        $json = $null
        if (-not [string]::IsNullOrWhiteSpace($raw)) {
            $json = $raw | ConvertFrom-Json
        }

        return [pscustomobject]@{
            StatusCode = [int]$response.StatusCode
            Body       = $json
            Raw        = $raw
        }
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

$runId = (Get-Date -Format "yyyyMMdd-HHmmss") + "-" + ([guid]::NewGuid().ToString("N").Substring(0, 8))
$rootDir = "/e2e-$runId"
$srcDir = "$rootDir/source"
$dstDir = "$rootDir/moved"
$cpyDir = "$rootDir/copied"

$localFileA = Join-Path $env:TEMP "e2e-a-$runId.txt"
$localFileB = Join-Path $env:TEMP "e2e-b-$runId.txt"

$adminAccess = $null
$adminRefresh = $null
$editorAccess = $null
$editorRefresh = $null
$createdUser = "editor_$($runId.Replace('-', '_'))"
$createdPassword = "P@ssw0rd!$runId"

$uploadedA = $null
$renamedA = $null
$movedA = $null
$copiedA = $null

try {
    Write-Step "Base URL: $BaseUrl"

    if (-not (Test-ApiHealth -Url $BaseUrl)) {
        throw "La API no responde en $BaseUrl. Inicia el servidor con: go run ./cmd/server"
    }

    Write-Step "1) Health"
    $health = Invoke-WebRequest -Method GET -Uri "$BaseUrl/health" -UseBasicParsing
    Assert-True ($health.StatusCode -eq 200) "Health endpoint falló"

    Write-Step "2) Auth Login (admin)"
    $loginAdmin = Invoke-JsonApi -Method "POST" -Url "$BaseUrl/api/v1/auth/login" -Body @{
        username = $AdminUsername
        password = $AdminPassword
    } -ExpectedStatus 200
    Assert-True ($loginAdmin.Body.success -eq $true) "Login admin devolvió success=false"

    $adminAccess = [string]$loginAdmin.Body.data.access_token
    $adminRefresh = [string]$loginAdmin.Body.data.refresh_token
    Assert-True (-not [string]::IsNullOrWhiteSpace($adminAccess)) "access_token admin vacío"
    Assert-True (-not [string]::IsNullOrWhiteSpace($adminRefresh)) "refresh_token admin vacío"

    $adminHeaders = @{ Authorization = "Bearer $adminAccess" }

    Write-Step "3) Auth Me"
    $meResp = Invoke-JsonApi -Method "GET" -Url "$BaseUrl/api/v1/auth/me" -Headers $adminHeaders -ExpectedStatus 200
    Assert-True ($meResp.Body.success -eq $true) "auth/me devolvió success=false"

    Write-Step "4) Auth Refresh"
    $refreshResp = Invoke-JsonApi -Method "POST" -Url "$BaseUrl/api/v1/auth/refresh" -Body @{ refresh_token = $adminRefresh } -ExpectedStatus 200
    Assert-True ($refreshResp.Body.success -eq $true) "auth/refresh devolvió success=false"

    $adminAccess = [string]$refreshResp.Body.data.access_token
    $adminRefresh = [string]$refreshResp.Body.data.refresh_token
    $adminHeaders = @{ Authorization = "Bearer $adminAccess" }

    Write-Step "5) Auth Register (editor)"
    $registerResp = Invoke-JsonApi -Method "POST" -Url "$BaseUrl/api/v1/auth/register" -Headers $adminHeaders -Body @{
        username = $createdUser
        password = $createdPassword
        role     = "editor"
    } -ExpectedStatus 201
    Assert-True ($registerResp.Body.success -eq $true) "auth/register devolvió success=false"

    Write-Step "6) Auth Login (editor)"
    $loginEditor = Invoke-JsonApi -Method "POST" -Url "$BaseUrl/api/v1/auth/login" -Body @{
        username = $createdUser
        password = $createdPassword
    } -ExpectedStatus 200
    Assert-True ($loginEditor.Body.success -eq $true) "Login editor devolvió success=false"

    $editorAccess = [string]$loginEditor.Body.data.access_token
    $editorRefresh = [string]$loginEditor.Body.data.refresh_token
    Assert-True (-not [string]::IsNullOrWhiteSpace($editorAccess)) "access_token editor vacío"
    $editorHeaders = @{ Authorization = "Bearer $editorAccess" }

    Write-Step "7) Crear estructura de directorios"
    $null = Invoke-JsonApi -Method "POST" -Url "$BaseUrl/api/v1/directories" -Headers $editorHeaders -Body @{ path = "/"; name = "e2e-$runId" } -ExpectedStatus 201
    $null = Invoke-JsonApi -Method "POST" -Url "$BaseUrl/api/v1/directories" -Headers $editorHeaders -Body @{ path = $rootDir; name = "source" } -ExpectedStatus 201
    $null = Invoke-JsonApi -Method "POST" -Url "$BaseUrl/api/v1/directories" -Headers $editorHeaders -Body @{ path = $rootDir; name = "moved" } -ExpectedStatus 201
    $null = Invoke-JsonApi -Method "POST" -Url "$BaseUrl/api/v1/directories" -Headers $editorHeaders -Body @{ path = $rootDir; name = "copied" } -ExpectedStatus 201

    Write-Step "8) Crear archivos locales y subir"
    "e2e content A $runId" | Set-Content -Path $localFileA -Encoding utf8
    "e2e content B $runId" | Set-Content -Path $localFileB -Encoding utf8

    $uploadA = Invoke-MultipartUpload -Url "$BaseUrl/api/v1/files/upload" -Token $editorAccess -DestinationPath $srcDir -FilePath $localFileA -ExpectedStatus 200
    $uploadB = Invoke-MultipartUpload -Url "$BaseUrl/api/v1/files/upload" -Token $editorAccess -DestinationPath $srcDir -FilePath $localFileB -ExpectedStatus 200
    Assert-True ($uploadA.Body.success -eq $true) "upload A devolvió success=false"
    Assert-True ($uploadB.Body.success -eq $true) "upload B devolvió success=false"

    $uploadedA = [string]$uploadA.Body.data.uploaded[0].path
    Assert-True (-not [string]::IsNullOrWhiteSpace($uploadedA)) "No se obtuvo path del archivo subido"

    Write-Step "9) Listado de directorio"
    $listUrl = "$BaseUrl/api/v1/files?path=$([uri]::EscapeDataString($srcDir))&page=1&limit=50&sort=name&order=asc"
    $listResp = Invoke-JsonApi -Method "GET" -Url $listUrl -Headers $editorHeaders -ExpectedStatus 200
    Assert-True ($listResp.Body.success -eq $true) "listado devolvió success=false"
    Assert-True (@($listResp.Body.data.items).Count -ge 2) "listado no contiene los archivos esperados"

    Write-Step "10) Metadata info"
    $infoUrl = "$BaseUrl/api/v1/files/info?path=$([uri]::EscapeDataString($uploadedA))"
    $infoResp = Invoke-JsonApi -Method "GET" -Url $infoUrl -Headers $editorHeaders -ExpectedStatus 200
    Assert-True ($infoResp.Body.success -eq $true) "files/info devolvió success=false"
    Assert-True ($infoResp.Body.data.type -eq "file") "files/info no reporta tipo file"

    Write-Step "11) Preview"
    $previewUrl = "$BaseUrl/api/v1/files/preview?path=$([uri]::EscapeDataString($uploadedA))"
    $previewResp = Invoke-WebRequest -Method GET -Uri $previewUrl -Headers $editorHeaders -ErrorAction Stop
    Assert-True ($previewResp.StatusCode -eq 200) "preview falló"
    Assert-True ($previewResp.Headers["Content-Disposition"] -like "inline*" ) "preview no devolvió Content-Disposition inline"

    Write-Step "12) Download archivo"
    $downloadFileUrl = "$BaseUrl/api/v1/files/download?path=$([uri]::EscapeDataString($uploadedA))"
    $downloadFileResp = Invoke-WebRequest -Method GET -Uri $downloadFileUrl -Headers $editorHeaders -ErrorAction Stop
    Assert-True ($downloadFileResp.StatusCode -eq 200) "download de archivo falló"
    Assert-True ($downloadFileResp.Headers["Content-Disposition"] -like "attachment*" ) "download no devolvió attachment"

    Write-Step "13) Download directorio ZIP"
    $downloadZipUrl = "$BaseUrl/api/v1/files/download?path=$([uri]::EscapeDataString($srcDir))&archive=true"
    $downloadZipResp = Invoke-WebRequest -Method GET -Uri $downloadZipUrl -Headers $editorHeaders -ErrorAction Stop
    Assert-True ($downloadZipResp.StatusCode -eq 200) "download ZIP falló"
    Assert-True ($downloadZipResp.Headers["Content-Type"] -like "application/zip*") "download ZIP no devolvió content-type application/zip"

    Write-Step "14) Rename"
    $renameResp = Invoke-JsonApi -Method "PUT" -Url "$BaseUrl/api/v1/files/rename" -Headers $editorHeaders -Body @{
        path = $uploadedA
        new_name = "renamed-$runId.txt"
    } -ExpectedStatus 200
    Assert-True ($renameResp.Body.success -eq $true) "rename devolvió success=false"
    $renamedA = [string]$renameResp.Body.data.new_path

    Write-Step "15) Move"
    $moveResp = Invoke-JsonApi -Method "PUT" -Url "$BaseUrl/api/v1/files/move" -Headers $editorHeaders -Body @{
        sources = @($renamedA)
        destination = $dstDir
    } -ExpectedStatus 200
    Assert-True ($moveResp.Body.success -eq $true) "move devolvió success=false"
    Assert-True (@($moveResp.Body.data.failed).Count -eq 0) "move reportó elementos fallidos"
    $movedA = [string]$moveResp.Body.data.moved[0].to

    Write-Step "16) Copy"
    $copyResp = Invoke-JsonApi -Method "POST" -Url "$BaseUrl/api/v1/files/copy" -Headers $editorHeaders -Body @{
        sources = @($movedA)
        destination = $cpyDir
    } -ExpectedStatus 200
    Assert-True ($copyResp.Body.success -eq $true) "copy devolvió success=false"
    Assert-True (@($copyResp.Body.data.failed).Count -eq 0) "copy reportó elementos fallidos"
    $copiedA = [string]$copyResp.Body.data.copied[0].to

    Write-Step "17) Search"
    $searchUrl = "$BaseUrl/api/v1/search?q=e2e&path=$([uri]::EscapeDataString($rootDir))&type=file&ext=.txt&page=1&limit=50"
    $searchResp = Invoke-JsonApi -Method "GET" -Url $searchUrl -Headers $editorHeaders -ExpectedStatus 200
    Assert-True ($searchResp.Body.success -eq $true) "search devolvió success=false"
    Assert-True (@($searchResp.Body.data.items).Count -ge 1) "search no devolvió resultados"

    Write-Step "18) Delete (archivo copiado)"
    $deleteCopy = Invoke-JsonApi -Method "DELETE" -Url "$BaseUrl/api/v1/files" -Headers $editorHeaders -Body @{ paths = @($copiedA) } -ExpectedStatus 200
    Assert-True ($deleteCopy.Body.success -eq $true) "delete archivo copiado devolvió success=false"

    Write-Step "19) Auth Logout (editor)"
    $logoutEditor = Invoke-JsonApi -Method "POST" -Url "$BaseUrl/api/v1/auth/logout" -Headers $editorHeaders -Body @{ refresh_token = $editorRefresh } -ExpectedStatus 200
    Assert-True ($logoutEditor.Body.success -eq $true) "logout editor devolvió success=false"

    Write-Step "20) Cleanup estructura de prueba"
    $cleanupResp = Invoke-JsonApi -Method "DELETE" -Url "$BaseUrl/api/v1/files" -Headers $adminHeaders -Body @{ paths = @($rootDir) } -ExpectedStatus 200
    Assert-True ($cleanupResp.Body.success -eq $true) "cleanup final devolvió success=false"

    Write-Step "21) Auth Logout (admin)"
    $logoutAdmin = Invoke-JsonApi -Method "POST" -Url "$BaseUrl/api/v1/auth/logout" -Headers $adminHeaders -Body @{ refresh_token = $adminRefresh } -ExpectedStatus 200
    Assert-True ($logoutAdmin.Body.success -eq $true) "logout admin devolvió success=false"

    Write-Host "`n[E2E] Todos los endpoints fueron verificados correctamente." -ForegroundColor Green
    Write-Host "[E2E] Usuario creado para prueba: $createdUser"
    $global:LASTEXITCODE = 0
}
catch {
    Write-Host "`n[E2E] FALLO: $($_.Exception.Message)" -ForegroundColor Red
    $global:LASTEXITCODE = 1
}
finally {
    if (Test-Path $localFileA) {
        Remove-Item -Path $localFileA -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path $localFileB) {
        Remove-Item -Path $localFileB -Force -ErrorAction SilentlyContinue
    }

    if ($PauseAtEnd) {
        Write-Host "`nPresiona Enter para cerrar..." -ForegroundColor DarkGray
        [void](Read-Host)
    }
}
