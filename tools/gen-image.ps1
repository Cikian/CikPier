# 直接调用 OpenAI 兼容的 /v1/images/generations 生成图片。
#
# 为什么要自己写一个：DSH 内置的生图工具在请求时会手动带上 Accept-Encoding: gzip，
# 而 Go 的 http.Transport **只在它自己添加这个头时**才自动解压。手动设了就会跳过解压，
# 于是拿到一堆原始 gzip 字节去当 JSON 解析，报
#   Unexpected token '', "� ..." is not valid JSON
# 这个脚本用 .NET 的 HttpClient，不主动声明 gzip，因此不会触发这个问题。
#
#   pwsh -File tools/gen-image.ps1 -Spec prompts.json -OutDir out
#
# 凭据从 DSH 的凭据库读（CIKHUB_API_KEY），不会打印出来。
param(
  [Parameter(Mandatory = $true)][string]$Spec,
  [Parameter(Mandatory = $true)][string]$OutDir,
  [string]$Model = 'gpt-image-2.5-sunburst',
  [string]$Size = '1024x1024',
  [string]$Quality = 'high',
  [int]$Concurrency = 3,
  [string]$BaseUrl = 'https://api.cikhub.com/v1'
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Net.Http

# ---- 取 key（不打印）----
$credPath = Join-Path $env:USERPROFILE '.dsh\.credentials.yaml'
if (-not (Test-Path $credPath)) { throw "找不到凭据文件：$credPath" }
$m = [regex]::Match((Get-Content $credPath -Raw), 'CIKHUB_API_KEY:\s*(\S+)')
if (-not $m.Success) { throw "凭据里没有 CIKHUB_API_KEY" }
$key = $m.Groups[1].Value.Trim('"', "'")

# ---- 读任务清单 ----
$jobs = Get-Content $Spec -Raw -Encoding UTF8 | ConvertFrom-Json
if (-not (Test-Path $OutDir)) { New-Item -ItemType Directory -Force -Path $OutDir | Out-Null }

$handler = New-Object System.Net.Http.HttpClientHandler
$client = New-Object System.Net.Http.HttpClient($handler)
$client.Timeout = [TimeSpan]::FromSeconds(600)
$client.DefaultRequestHeaders.Authorization =
  New-Object System.Net.Http.Headers.AuthenticationHeaderValue('Bearer', $key)

$url = "$BaseUrl/images/generations"
Write-Output ("模型 $Model  尺寸 $Size  质量 $Quality  共 " + $jobs.Count + " 张")

$total = $jobs.Count
$index = 0
$failed = 0

# 分批发，避免一次性打太多请求
for ($start = 0; $start -lt $total; $start += $Concurrency) {
  $end = [Math]::Min($start + $Concurrency - 1, $total - 1)
  $batch = @()
  for ($i = $start; $i -le $end; $i++) {
    $j = $jobs[$i]
    $payload = @{
      model   = $Model
      prompt  = $j.prompt
      n       = 1
      size    = $Size
      quality = $Quality
    } | ConvertTo-Json -Compress
    $content = New-Object System.Net.Http.StringContent($payload, [System.Text.Encoding]::UTF8, 'application/json')
    $batch += [pscustomobject]@{ name = $j.name; task = $client.PostAsync($url, $content) }
  }

  foreach ($b in $batch) {
    $index++
    try {
      $resp = $b.task.GetAwaiter().GetResult()
      if (-not $resp.IsSuccessStatusCode) {
        $err = $resp.Content.ReadAsStringAsync().Result
        Write-Output ("  [{0}/{1}] {2}  ❌ HTTP {3}  {4}" -f $index, $total, $b.name, [int]$resp.StatusCode, $err.Substring(0, [Math]::Min(200, $err.Length)))
        $failed++
        continue
      }
      $json = $resp.Content.ReadAsStringAsync().Result | ConvertFrom-Json
      $b64 = $json.data[0].b64_json
      if (-not $b64) {
        Write-Output ("  [{0}/{1}] {2}  ❌ 响应里没有 b64_json" -f $index, $total, $b.name)
        $failed++
        continue
      }
      $out = Join-Path $OutDir ("{0}.png" -f $b.name)
      [System.IO.File]::WriteAllBytes($out, [Convert]::FromBase64String($b64))
      $kb = [math]::Round((Get-Item $out).Length / 1KB)
      Write-Output ("  [{0}/{1}] {2}  ✅ {3} KB" -f $index, $total, $b.name, $kb)
    } catch {
      Write-Output ("  [{0}/{1}] {2}  ❌ {3}" -f $index, $total, $b.name, $_.Exception.Message)
      $failed++
    }
  }
}

Write-Output ""
if ($failed -gt 0) { Write-Output ("完成，失败 $failed 张") } else { Write-Output "全部完成" }
