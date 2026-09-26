param([string]$In)
Add-Type -AssemblyName System.Drawing
$img = [System.Drawing.Image]::FromFile($In)
$bmp = New-Object System.Drawing.Bitmap $img
$minX = 99999; $maxX = -1; $minY = 99999; $maxY = -1; $count = 0
for ($y = 0; $y -lt $bmp.Height; $y++) {
  for ($x = 0; $x -lt $bmp.Width; $x++) {
    $c = $bmp.GetPixel($x, $y)
    if ($c.R -gt 200 -and $c.G -gt 90 -and $c.G -lt 200 -and $c.B -lt 110) {
      $count++
      if ($x -lt $minX) { $minX = $x }
      if ($x -gt $maxX) { $maxX = $x }
      if ($y -lt $minY) { $minY = $y }
      if ($y -gt $maxY) { $maxY = $y }
    }
  }
}
Write-Output ("image          : " + $bmp.Width + "x" + $bmp.Height)
Write-Output ("orange pixels  : " + $count)
if ($count -gt 0) {
  Write-Output ("orange bbox    : x " + $minX + ".." + $maxX + "   y " + $minY + ".." + $maxY)
}
# 顺带找出卡片右边界：在按钮所在那一行上，从右往左找第一个明显不是页面底色的像素
if ($count -gt 0) {
  $midY = [int](($minY + $maxY) / 2)
  Write-Output ("--- 扫描 y=" + $midY + " 的右侧边界 ---")
  for ($x = $bmp.Width - 1; $x -ge $bmp.Width - 60; $x--) {
    $c = $bmp.GetPixel($x, $midY)
    Write-Output ("  x=" + $x + "  rgb(" + $c.R + "," + $c.G + "," + $c.B + ")")
  }
}
$bmp.Dispose(); $img.Dispose()
