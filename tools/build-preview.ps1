# 把 app/frontend/src 的界面代码同步到 design/preview，并注入假后端，
# 这样可以直接用浏览器打开 preview.html 出图（不需要启动程序，也不需要 frp）。
#
#   powershell -File tools/build-preview.ps1
#   powershell -File tools/build-preview.ps1 -Shots      # 顺便用 Edge 截图
param(
  [switch]$Shots,
  [string]$Edge = 'C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe'
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$src  = Join-Path $root 'app\frontend\src'
$dst  = Join-Path $root 'design\preview'
# 截图统一放这里，别混在 preview 里。
# ⚠ 变量名不能叫 $shots —— PowerShell 变量名**大小写不敏感**，
#   会和上面的 [switch]$Shots 参数撞名，赋值时直接报类型错误。
$shotDir = Join-Path $root 'design\screenshots'

if (-not (Test-Path $src)) { throw "找不到 $src" }
if (-not (Test-Path $dst)) { New-Item -ItemType Directory -Force -Path $dst | Out-Null }

# 1) 同步界面代码与资源（preview 目录里的 mock.js 保留）
foreach ($f in 'main.css', 'main.js', 'index.html') {
  Copy-Item (Join-Path $src $f) (Join-Path $dst $f) -Force
}
$assetsSrc = Join-Path $src 'assets'
$assetsDst = Join-Path $dst 'assets'
if (Test-Path $assetsDst) { Remove-Item $assetsDst -Recurse -Force }
Copy-Item $assetsSrc $assetsDst -Recurse -Force

# 2) 生成 preview.html：在 main.js 之前插入 mock.js
$html = Get-Content (Join-Path $src 'index.html') -Raw -Encoding UTF8
$html = $html.Replace('<script src="main.js"></script>',
                      '<script src="mock.js"></script>' + "`r`n" + '<script src="main.js"></script>')
# 用 .NET 写，保证 UTF-8 无 BOM（PS 5.1 没有 utf8NoBOM 这个枚举）
[System.IO.File]::WriteAllText((Join-Path $dst 'preview.html'), $html, (New-Object System.Text.UTF8Encoding($false)))
Write-Output "已生成 $dst\preview.html"

# 3) 可选：用 Edge 无头模式截图
if ($Shots) {
  if (-not (Test-Path $Edge)) { throw "找不到 Edge: $Edge" }
  $base = 'file:///' + ($dst -replace '\\', '/') + '/preview.html'
  if (-not (Test-Path $shotDir)) { New-Item -ItemType Directory -Force -Path $shotDir | Out-Null }
  $pages = @('proxies', 'logs', 'settings', 'visitors', 'diagnose', 'about',
             'modal-tcp', 'modal-http', 'modal-stcp', 'wizard', 'donate', 'panel')
  foreach ($p in $pages) {
    $out = Join-Path $shotDir "shot-$p.png"
    Remove-Item $out -ErrorAction SilentlyContinue
    $eargs = @('--headless=new', '--disable-gpu', '--no-sandbox', '--hide-scrollbars',
               '--window-size=1180,760', "--user-data-dir=$env:TEMP\edgeshot-$p",
               '--virtual-time-budget=8000', '--run-all-compositor-stages-before-draw',
               "--screenshot=$out", "$base#$p")
    Start-Process -FilePath $Edge -ArgumentList $eargs -Wait -NoNewWindow
    if (Test-Path $out) { Write-Output ("  OK   {0,-12} {1} bytes" -f $p, (Get-Item $out).Length) }
    else { Write-Output ("  FAIL {0}" -f $p) }
  }
}
