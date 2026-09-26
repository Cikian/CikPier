param([string]$Exe, [string]$Out)
Add-Type -AssemblyName System.Drawing
$code = @"
using System;
using System.Drawing;
public class ExeIcon {
  public static void Dump(string exe, string outPng) {
    using (Icon ic = Icon.ExtractAssociatedIcon(exe)) {
      using (Bitmap bmp = ic.ToBitmap()) {
        bmp.Save(outPng, System.Drawing.Imaging.ImageFormat.Png);
      }
    }
  }
}
"@
Add-Type -TypeDefinition $code -ReferencedAssemblies System.Drawing
[ExeIcon]::Dump($Exe, $Out)
Write-Output ("saved " + $Out)
