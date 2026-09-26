param([string]$In, [int]$Y)
Add-Type -AssemblyName System.Drawing
$img = [System.Drawing.Image]::FromFile($In)
$bmp = New-Object System.Drawing.Bitmap $img
Write-Output ("image: " + $bmp.Width + "x" + $bmp.Height + "  scanline y=" + $Y)
$prev = $null
$runs = @()
$start = 0
for ($x = 0; $x -lt $bmp.Width; $x++) {
  $c = $bmp.GetPixel($x, $Y)
  # 量化到 16 级，忽略抗锯齿的细微渐变
  $key = "{0}-{1}-{2}" -f [int]($c.R/16), [int]($c.G/16), [int]($c.B/16)
  if ($key -ne $prev) {
    if ($prev -ne $null) { $runs += ("{0,5}..{1,-5} {2}" -f $start, ($x-1), $prev) }
    $prev = $key; $start = $x
  }
}
$runs += ("{0,5}..{1,-5} {2}" -f $start, ($bmp.Width-1), $prev)
$runs | ForEach-Object { Write-Output $_ }
$bmp.Dispose(); $img.Dispose()
