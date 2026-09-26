param(
  [Parameter(Mandatory = $true)][string]$Dir,
  [string]$Out,
  [string]$Exclude = '对比图|尺寸'
)
Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName System.Drawing.Drawing2D

$files = Get-ChildItem $Dir -Filter '*.png' -File |
         Where-Object { $_.BaseName -notmatch $Exclude } |
         Sort-Object Name
if ($files.Count -eq 0) { throw "没有可用图片：$Dir" }
if (-not $Out) { $Out = Join-Path $Dir '尺寸测试.png' }

# 每个尺寸用不同的放大倍数，让最终占的宽度接近，便于对比
$sizes = @(
  @{ px = 16; zoom = 8 },
  @{ px = 32; zoom = 5 },
  @{ px = 48; zoom = 4 },
  @{ px = 64; zoom = 3 }
)
$labelW = 190
$gap = 16
$rowH = 140

$w = $labelW + $gap + ($sizes | ForEach-Object { $_.px * $_.zoom + $gap } | Measure-Object -Sum).Sum
$h = $gap + $files.Count * ($rowH + $gap) + 40

$canvas = New-Object System.Drawing.Bitmap ([int]$w), ([int]$h)
$g = [System.Drawing.Graphics]::FromImage($canvas)
$g.Clear([System.Drawing.Color]::FromArgb(247, 249, 251))
$g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
$g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality

$font = New-Object System.Drawing.Font('Microsoft YaHei', 12, [System.Drawing.FontStyle]::Bold)
$small = New-Object System.Drawing.Font('Microsoft YaHei', 10)
$brush = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(15, 23, 42))
$muted = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(100, 116, 139))

# 表头
$x = $labelW + $gap
foreach ($s in $sizes) {
  $g.DrawString("$($s.px)px", $small, $muted, [single]$x, [single]4)
  $x += $s.px * $s.zoom + $gap
}

for ($i = 0; $i -lt $files.Count; $i++) {
  $y = $gap + 30 + $i * ($rowH + $gap)
  $g.DrawString($files[$i].BaseName, $font, $brush, [single]8, [single]($y + 40))

  $src = [System.Drawing.Image]::FromFile($files[$i].FullName)
  $x = $labelW + $gap
  foreach ($s in $sizes) {
    $tmp = New-Object System.Drawing.Bitmap $s.px, $s.px
    $tg = [System.Drawing.Graphics]::FromImage($tmp)
    $tg.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $tg.DrawImage($src, 0, 0, $s.px, $s.px)
    $tg.Dispose()
    $g.DrawImage($tmp, $x, $y, $s.px * $s.zoom, $s.px * $s.zoom)
    $tmp.Dispose()
    $x += $s.px * $s.zoom + $gap
  }
  $src.Dispose()
}

$g.Dispose()
$canvas.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
$canvas.Dispose()
Write-Output ("尺寸测试图 -> " + $Out + "  (" + [int]$w + "x" + [int]$h + ")")
