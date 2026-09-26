param(
  [string]$Src,
  [string]$Dst,
  [int]$Size = 512,
  [int]$Quality = 92
)
Add-Type -AssemblyName System.Drawing
$code = @"
using System;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Drawing.Imaging;
public class ImgScale {
  // 等比缩放到边长 Size 的正方形里，不裁剪。
  // 已经是正方形的原图就等于整体缩放；不是正方形的话会留白（保持比例，绝不切掉内容）。
  public static void Fit(string src, string dst, int size, long quality) {
    using (Image img = Image.FromFile(src)) {
      using (Bitmap bmp = new Bitmap(size, size))
      {
        using (Graphics g = Graphics.FromImage(bmp)) {
          g.InterpolationMode = InterpolationMode.HighQualityBicubic;
          g.PixelOffsetMode = PixelOffsetMode.HighQuality;
          g.SmoothingMode = SmoothingMode.HighQuality;
          g.Clear(Color.White);
          double scale = Math.Min((double)size / img.Width, (double)size / img.Height);
          int w = (int)Math.Round(img.Width * scale);
          int h = (int)Math.Round(img.Height * scale);
          g.DrawImage(img, new Rectangle((size - w) / 2, (size - h) / 2, w, h));
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
[ImgScale]::Fit($Src, $Dst, $Size, $Quality)
Write-Output ("fit -> " + $Dst)
