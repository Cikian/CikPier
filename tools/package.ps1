# 一键打包：编译 -> 收集文件 -> 打 zip
#
#   powershell -ExecutionPolicy Bypass -File tools/package.ps1 -Version 1.0.0
#       默认**一次出两个包**：x64 和 Windows on ARM
#
#   powershell -ExecutionPolicy Bypass -File tools/package.ps1 -Version 1.0.0 -Arch amd64
#       只出 x64（调试用）
#   powershell -ExecutionPolicy Bypass -File tools/package.ps1 -SkipBuild -Arch arm64
#       只重打包，不重新编译（必须指定单一架构，见下方校验）
#
# 产物 —— 两个都带架构后缀，落在工作区根的 release\：
#   release\CikPier-<版本>-amd64.zip
#   release\CikPier-<版本>-arm64.zip
#
# 打包内容就是 dist-staging/ 里的所有东西（再把对应架构的 exe 拷进去）。
# 想往里加文件（比如额外的说明文档），直接丢进 dist-staging/ 就行，这个脚本不会删它。
#
# ⚠ 关于架构：x64 的包在 Windows on ARM 上**能靠模拟跑，但不推荐** ——
#   WebView2 在 ARM64 上跑 x64 目标程序时，发大消息会死锁甚至崩溃
#   （MicrosoftEdge/WebView2Feedback#4589，至今未关）。所以要给 ARM 用户单独出包。
param(
  [string]$Version = '',
  [ValidateSet('amd64', 'arm64', 'both')][string]$Arch = 'both',
  [switch]$SkipBuild,
  [switch]$RefreshFrpc,
  [string]$FrpcPath = '',
  [string]$FrpVersion = '0.71.0'
)

$ErrorActionPreference = 'Stop'
$root   = Split-Path -Parent $PSScriptRoot          # cikpier/
$app    = Join-Path $root 'app'
$stage  = Join-Path $root 'dist-staging'
$outDir = Join-Path (Split-Path -Parent $root) 'release'   # 所有分发包集中在这里

# 官方二进制（frpc.exe / frps.exe）放哪。支持两种布局：
#   1) 单独克隆本仓库 —— <仓库>/frp-bins/
#   2) 我的本地工作区 —— <工作区>/frp-bins/（本仓库是工作区里的一个子目录）
# 优先用仓库内的那份，这样别人把仓库 clone 下来也能直接打包。
# 仓库内没有时再退回工作区那份，本地开发行为和以前完全一致。
$binsInRepo = Join-Path $root 'frp-bins'
$bins = if (Test-Path $binsInRepo) { $binsInRepo }
        else { Join-Path (Split-Path -Parent $root) 'frp-bins' }

$exeName = 'CikPier.exe'

# ------------------------------------------------ 参数组合校验
# -SkipBuild / -FrpcPath 都只对「单一架构」成立：
#   build\bin 里一次只躺着一个架构的产物，跳过编译就不可能出两个包；
#   一份 frpc.exe 也只对应一个架构。
if ($Arch -eq 'both') {
  if ($SkipBuild) {
    throw "-SkipBuild 必须配合 -Arch amd64 或 -Arch arm64。`n" +
          "build\bin 里一次只有一个架构的产物，跳过编译没法出两个包。`n" +
          "例如：-SkipBuild -Arch arm64"
  }
  if ($FrpcPath) {
    throw "-FrpcPath 必须配合 -Arch amd64 或 -Arch arm64（一份 frpc.exe 只对应一个架构）。"
  }
}

if (-not (Test-Path $outDir)) { New-Item -ItemType Directory -Force -Path $outDir | Out-Null }

# 界面上显示的版本号写在 app/about.go 里，别和压缩包名字对不上
$appVersion = ''
$aboutFile = Join-Path $app 'about.go'
if (Test-Path $aboutFile) {
  $m = [regex]::Match((Get-Content $aboutFile -Raw -Encoding UTF8), 'appVersion\s*=\s*"([^"]+)"')
  if ($m.Success) { $appVersion = $m.Groups[1].Value }
}
if ($appVersion) {
  if ($Version -and $Version -ne $appVersion) {
    Write-Warning "包名写的是 v$Version，但 about.go 里是 v$appVersion —— 要同步的话请改 app\about.go。"
  }
  if (-not $Version) {
    Write-Warning "没传 -Version，包名里就没有版本号。建议：-Version $appVersion"
  }
}

# ---------------------------------------------------------------- 工具函数
function Get-PEArchName([string]$path) {
  $b = [System.IO.File]::ReadAllBytes($path)
  $pe = [BitConverter]::ToInt32($b, 0x3C)
  $m = [BitConverter]::ToUInt16($b, $pe + 4)
  switch ($m) { 0x014c { 'x86' } 0x8664 { 'amd64' } 0xAA64 { 'arm64' } default { "0x{0:X}" -f $m } }
}

# arm64 的官方二进制本地可能没有，按需从 GitHub Releases 拉一次
function Ensure-Arm64Frpc([string]$ver) {
  $dir = Join-Path $bins 'win-arm64'
  $exe = Join-Path $dir 'frpc.exe'
  if (Test-Path $exe) { return $exe }
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  $url = "https://github.com/fatedier/frp/releases/download/v$ver/frp_${ver}_windows_arm64.zip"
  $tmp = Join-Path $env:TEMP "frp_${ver}_windows_arm64.zip"
  Write-Host "      本地没有 arm64 版 frpc.exe，从官方下载：$url" -ForegroundColor DarkYellow
  $ProgressPreference = 'SilentlyContinue'
  Invoke-WebRequest -Uri $url -OutFile $tmp -TimeoutSec 600
  $dst = Join-Path $env:TEMP "frp_${ver}_windows_arm64"
  Remove-Item $dst -Recurse -Force -ErrorAction SilentlyContinue
  Expand-Archive -Path $tmp -DestinationPath $dst -Force
  $found = Get-ChildItem $dst -Recurse -Filter 'frpc.exe' | Select-Object -First 1
  if (-not $found) { throw "从下载的包里找不到 frpc.exe" }
  Copy-Item $found.FullName $exe -Force
  foreach ($n in 'frps.exe') {
    $f = Get-ChildItem $dst -Recurse -Filter $n | Select-Object -First 1
    if ($f) { Copy-Item $f.FullName (Join-Path $dir $n) -Force }
  }
  return $exe
}

# ---------------------------------------------------------------- 单架构打包
function Invoke-ArchPackage([string]$TargetArch) {
  $plat = "windows/$TargetArch"
  Write-Host ""
  Write-Host "================ $TargetArch ================" -ForegroundColor Cyan

  # ------------------------------------------------------------ 1) 编译
  if (-not $SkipBuild) {
    Write-Host "[1/4] 编译（wails build -platform $plat）..." -ForegroundColor Yellow

    # ⚠ `wails build -clean` 会先删 build\bin。如果那里还有进程在跑
    #   （典型情况：上次 wails dev 顺手拉起的 frpc.exe），删除会失败，
    #   而报错是 "unlinkat ...: Access is denied."，完全看不出是进程占用。
    #   所以先把占用者清掉。
    $busy = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
            Where-Object { $_.ExecutablePath -like "$app\build\bin\*" }
    if ($busy) {
      $names = ($busy | ForEach-Object { "$($_.Name)($($_.ProcessId))" }) -join ', '
      Write-Host "      清理占用 build\bin 的进程：$names" -ForegroundColor DarkYellow
      foreach ($proc in $busy) { Stop-Process -Id $proc.ProcessId -Force -ErrorAction SilentlyContinue }
      Start-Sleep -Milliseconds 900
    }

    Push-Location $app
    try {
      # ⚠ 必须用 wails build。直接 go build 不带 -tags production 会得到一个
      #   什么都不做的空壳程序（静默退出，退出码 0），非常难查。
      # -o 指定输出名，避免交叉编译时 Wails 自动加架构后缀。
      & wails build -clean -skipbindings -platform $plat -o $exeName
      if ($LASTEXITCODE -ne 0) { throw "wails build 失败（退出码 $LASTEXITCODE）" }
    } finally {
      Pop-Location
    }
  } else {
    Write-Host "[1/4] 跳过编译" -ForegroundColor DarkGray
  }

  # ------------------------------------------------------------ 2) 收集
  $builtExe = Join-Path $app "build\bin\$exeName"
  if (-not (Test-Path $builtExe)) { throw "找不到编译产物：$builtExe" }
  if (-not (Test-Path $stage)) { New-Item -ItemType Directory -Force -Path $stage | Out-Null }

  Write-Host "[2/4] 收集文件到 dist-staging ..." -ForegroundColor Yellow
  Copy-Item $builtExe (Join-Path $stage $exeName) -Force

  # 清掉改名/换架构后留下的旧二进制，否则包里会同时躺着两个主程序
  Get-ChildItem $stage -Filter '*.exe' -File | Where-Object {
    $_.Name -ne $exeName -and $_.Name -notlike 'frpc*'
  } | ForEach-Object {
    Write-Host ("      移除过期的 " + $_.Name) -ForegroundColor DarkYellow
    Remove-Item $_.FullName -Force
  }
  # frpc 也要保证是当前架构那一个
  Get-ChildItem $stage -Filter 'frpc*.exe' -File | ForEach-Object {
    if ($_.Name -ne 'frpc.exe') { Remove-Item $_.FullName -Force }
  }

  # ------------------------------------------------------------ frpc.exe
  # 官方原版二进制，和界面程序分开发布（方便单独升级 frp）。必须和主程序同架构。
  #
  # ⚠ 这里必须**按架构**判断要不要重拷：连跑 amd64 + arm64 时，第二个架构看到的
  #   dist-staging\frpc.exe 是上一个架构留下的。只看「文件在不在」就会沿用错的那份，
  #   然后被下面的架构校验拦下（报错点在很后面，不好查）。
  $stageFrpc = Join-Path $stage 'frpc.exe'
  $needCopy = $true
  if ((Test-Path $stageFrpc) -and -not $RefreshFrpc) {
    if ((Get-PEArchName $stageFrpc) -eq $TargetArch) { $needCopy = $false }
  }

  if ($needCopy) {
    $candidates = @()
    if ($FrpcPath) { $candidates += $FrpcPath }
    if ($TargetArch -eq 'arm64') {
      $candidates += (Join-Path $bins 'win-arm64\frpc.exe')
    } else {
      $candidates += @(
        (Join-Path $bins 'win-amd64\frpc.exe'),
        (Join-Path $root '..\packages\frpc-客户端部署包\windows\frpc.exe')
      )
    }
    $found = $candidates | Where-Object { Test-Path $_ } | Select-Object -First 1

    if (-not $found -and $TargetArch -eq 'arm64') {
      $found = Ensure-Arm64Frpc $FrpVersion
    }
    if (-not $found) {
      throw "找不到 $TargetArch 版的 frpc.exe。请用 -FrpcPath 指定，或手动放进 dist-staging\。`n已尝试：`n  " + ($candidates -join "`n  ")
    }
    Copy-Item $found $stageFrpc -Force
    Write-Host ("      frpc.exe 来自 " + (Resolve-Path $found)) -ForegroundColor DarkGray
  } else {
    Write-Host "      dist-staging 里的 frpc.exe 已是 $TargetArch，沿用" -ForegroundColor DarkGray
  }

  # ------------------------------------------------------------ 3) 自检
  Write-Host "[3/4] 检查包内容 ..." -ForegroundColor Yellow
  $required = @($exeName, 'frpc.exe', '使用说明.md')
  foreach ($f in $required) {
    $p = Join-Path $stage $f
    if (-not (Test-Path $p)) { throw "包里缺少必需文件：$f" }
  }

  # ⚠ 架构必须对上：主程序和 frpc.exe 是同一套架构才能配合工作
  $mainArch = Get-PEArchName (Join-Path $stage $exeName)
  $frpcArch = Get-PEArchName (Join-Path $stage 'frpc.exe')
  if ($mainArch -ne $TargetArch) { throw "主程序架构是 $mainArch，和要求的 $TargetArch 不一致" }
  if ($frpcArch -ne $TargetArch) { throw "frpc.exe 架构是 $frpcArch，和主程序（$mainArch）不一致" }
  Write-Host "      架构校验通过：主程序 $mainArch / frpc.exe $frpcArch" -ForegroundColor DarkGray

  $files = Get-ChildItem $stage -File
  foreach ($f in $files) {
    Write-Host ("      {0,-18} {1,8:N0} KB" -f $f.Name, ($f.Length / 1KB))
  }

  # ------------------------------------------------------------ 4) 打 zip
  # 两个架构都带后缀，不再有「amd64 裸名」那种容易和 arm64 混淆的包
  $name = 'CikPier'
  if ($Version) { $name = "$name-v$Version" }
  $name = "$name-$TargetArch"
  $zip = Join-Path $outDir "$name.zip"

  Write-Host "[4/4] 打包 ..." -ForegroundColor Yellow

  # ⚠ 覆盖上一次的产物时有两个坑，都会报出和真实原因无关的错：
  #
  #   1) Compress-Archive 不带 -Force 时，文件已存在就直接失败，
  #      报 "The archive file ... already exists"，像是脚本写错了。
  #   2) 上一版 zip 被别的程序**打开着**（最常见是 7-Zip / WinRAR /
  #      资源管理器预览窗格）时，连删都删不掉。这时 Remove-Item 会被
  #      -ErrorAction SilentlyContinue 吞掉，然后卡在第 1 条的报错上 ——
  #      真正的原因（文件被占用）一个字都不显示。
  #
  # 所以这里先删、**删完verify**，删不掉就直说是占用问题。
  if (Test-Path $zip) {
    Remove-Item $zip -Force -ErrorAction SilentlyContinue
    if (Test-Path $zip) {
      throw @"
输出文件被别的程序占着，覆盖不了：
  $zip

多半是你正用压缩软件（7-Zip / WinRAR / Bandizip…）或资源管理器的预览窗格打开着它。
把它们关掉，然后重新跑一次就行 —— 不需要改任何配置。
"@
    }
  }
  Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip -CompressionLevel Optimal -Force

  $info = Get-Item $zip
  Write-Host ("      -> " + $info.FullName + "   " + [math]::Round($info.Length / 1MB, 2) + " MB") -ForegroundColor Green
}

# ---------------------------------------------------------------- 主流程
$targets = if ($Arch -eq 'both') { @('amd64', 'arm64') } else { @($Arch) }

Write-Host ("== CikPier 打包：" + ($targets -join ' + ') + " ==") -ForegroundColor Cyan
if ($appVersion) { Write-Host ("   （界面里显示的版本：v" + $appVersion + "）") -ForegroundColor DarkGray }

# ⚠ 逐个架构独立处理：一个失败不拖累另一个。
#   典型场景：amd64 的旧包正被 7-Zip 打开着覆盖不了，但 arm64 照样能出。
$failed = @()
foreach ($a in $targets) {
  try {
    Invoke-ArchPackage $a
  } catch {
    $failed += $a
    Write-Host ""
    Write-Host ("[失败] $a —— " + $_.Exception.Message) -ForegroundColor Red
  }
}

# 清掉旧命名（amd64 曾经不带后缀）留下的包，免得和新产物混淆
if ($Version) {
  $legacy = Join-Path $outDir "CikPier-v$Version.zip"
  if (Test-Path $legacy) {
    Remove-Item $legacy -Force
    Write-Host ("`n已移除旧命名产物 " + (Split-Path $legacy -Leaf) + "（现在两个包都带架构后缀）") -ForegroundColor DarkYellow
  }
}

# ---------------------------------------------------------------- 汇总
Write-Host ""
if ($failed.Count -gt 0) {
  Write-Host "================ 没出全 ================" -ForegroundColor Red
  Write-Host ("  失败：" + ($failed -join '、') + "（原因见上面）") -ForegroundColor Red
} else {
  Write-Host "================ 完成 ================" -ForegroundColor Green
}
Get-ChildItem $outDir -Filter 'CikPier*.zip' | Sort-Object Name | ForEach-Object {
  Write-Host ("  {0,-32} {1,7:N2} MB" -f $_.Name, ($_.Length / 1MB)) -ForegroundColor Green
}
Write-Host ("`ndist-staging\ 里留下的是最后处理的 " + $targets[-1] + " 那份（以 zip 为准）") -ForegroundColor DarkGray

if ($failed.Count -gt 0) { exit 1 }
