param([int]$ProcId, [string]$Out)
Add-Type -AssemblyName System.Drawing
$code = @"
using System;
using System.Drawing;
using System.Runtime.InteropServices;
public class WinCap {
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr hdc, uint flags);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
  public static Bitmap Grab(IntPtr h) {
    RECT r; GetWindowRect(h, out r);
    int w = r.Right - r.Left, ht = r.Bottom - r.Top;
    if (w <= 0 || ht <= 0) return null;
    Bitmap bmp = new Bitmap(w, ht);
    using (Graphics g = Graphics.FromImage(bmp)) {
      IntPtr hdc = g.GetHdc();
      PrintWindow(h, hdc, 2);
      g.ReleaseHdc(hdc);
    }
    return bmp;
  }
}
"@
Add-Type -TypeDefinition $code -ReferencedAssemblies System.Drawing
$p = Get-Process -Id $ProcId
$h = $p.MainWindowHandle
if ($h -eq 0) { Write-Output "no main window"; exit 1 }
[WinCap]::SetForegroundWindow($h) | Out-Null
Start-Sleep -Milliseconds 900
$bmp = [WinCap]::Grab($h)
if ($bmp -eq $null) { Write-Output "capture failed"; exit 1 }
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
Write-Output ("saved " + $Out)
