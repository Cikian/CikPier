param([string]$Src, [string]$Dst, [int]$Cx, [int]$Cy, [int]$Side, [int]$OutSize, [int]$Quality)
Add-Type -AssemblyName System.Drawing
$code = @"
using System;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Drawing.Imaging;
public class ImgTool {
  public static void CropSquare(string src, string dst, int cx, int cy, int side, int outSize, long quality) {
    using (Image img = Image.FromFile(src)) {
      int half = side / 2;
      int x = cx - half, y = cy - half;
      if (x < 0) x = 0;
      if (y < 0) y = 0;
      if (x + side > img.Width)  x = img.Width  - side;
      if (y + side > img.Height) y = img.Height - side;
      using (Bitmap bmp = new Bitmap(outSize, outSize))
      {
        using (Graphics g = Graphics.FromImage(bmp)) {
          g.InterpolationMode = InterpolationMode.HighQualityBicubic;
          g.PixelOffsetMode = PixelOffsetMode.HighQuality;
          g.SmoothingMode = SmoothingMode.HighQuality;
          g.DrawImage(img, new Rectangle(0, 0, outSize, outSize), new Rectangle(x, y, side, side), GraphicsUnit.Pixel);
        }
        ImageCodecInfo codec = null;
        foreach (ImageCodecInfo c in ImageCodecInfo.GetImageEncoders()) { if (c.MimeType == "image/jpeg") { codec = c; break; } }
        using (EncoderParameters ep = new EncoderParameters(1)) {
          ep.Param[0] = new EncoderParameter(Encoder.Quality, quality);
          bmp.Save(dst, codec, ep);
        }
      }
    }
  }
}
"@
Add-Type -TypeDefinition $code -ReferencedAssemblies System.Drawing
[ImgTool]::CropSquare($Src, $Dst, $Cx, $Cy, $Side, $OutSize, $Quality)
Write-Output ("done -> " + $Dst)
