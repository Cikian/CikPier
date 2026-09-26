param(
  [Parameter(Mandatory = $true)][string]$Dir,
  [string]$Filter = '*.png',
  [string]$Out,
  [string]$Exclude = '对比图',
  [int]$Cols = 3,
  [int]$Cell = 380,
  [int]$Pad = 18,
  [int]$LabelHeight = 30
)
Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName System.Drawing.Drawing2D

# 自己扫目录取文件：用 powershell.exe -File 传数组参数很别扭（会被当字符串拆开），
# 所以改成传目录 + 过滤器。
$files = Get-ChildItem $Dir -Filter $Filter -File |
         Where-Object { $_.BaseName -notlike "*$Exclude*" } |
         Sort-Object Name
if ($files.Count -eq 0) { throw "目录里没有匹配 $Filter 的文件：$Dir" }
if (-not $Out) { $Out = Join-Path $Dir '对比图.png' }

$rows = [Math]::Ceiling($files.Count / $Cols)
$w = $Pad + $Cols * ($Cell + $Pad)
$h = $Pad + $rows * ($Cell + $LabelHeight + $Pad)

$canvas = New-Object System.Drawing.Bitmap $w, $h
$g = [System.Drawing.Graphics]::FromImage($canvas)
$g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
$g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
$g.Clear([System.Drawing.Color]::FromArgb(247, 249, 251))

$font = New-Object System.Drawing.Font('Microsoft YaHei', 13, [System.Drawing.FontStyle]::Bold)
$brush = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(15, 23, 42))
$border = New-Object System.Drawing.Pen ([System.Drawing.Color]::FromArgb(203, 213, 225)), 2

for ($i = 0; $i -lt $files.Count; $i++) {
  $col = $i % $Cols
  $row = [Math]::Floor($i / $Cols)
  $x = $Pad + $col * ($Cell + $Pad)
  $y = $Pad + $row * ($Cell + $LabelHeight + $Pad)

  $img = [System.Drawing.Image]::FromFile($files[$i].FullName)
  $scale = [Math]::Min($Cell / $img.Width, $Cell / $img.Height)
  $dw = [int]($img.Width * $scale)
  $dh = [int]($img.Height * $scale)
  $dx = $x + [int](($Cell - $dw) / 2)
  $dy = $y + [int](($Cell - $dh) / 2)
  $g.DrawImage($img, $dx, $dy, $dw, $dh)
  $g.DrawRectangle($border, $dx, $dy, $dw, $dh)
  $img.Dispose()

  # 用文件名当标签（已按名称排序，天然带序号）
  $g.DrawString($files[$i].BaseName, $font, $brush, [single]$x, [single]($y + $Cell + 6))
}

$g.Dispose()
$canvas.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
$canvas.Dispose()
Write-Output ("拼图 -> " + $Out + "  (" + $w + "x" + $h + "，共 " + $files.Count + " 张)")
