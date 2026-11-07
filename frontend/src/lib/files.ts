/** blobToBase64 reads a dropped/pasted File or a recorded Blob as plain base64 (no data: prefix),
 * for inbox.sendFile({ dataBase64 }) and inbox.sendVoice({ dataBase64 }). */
export function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onerror = () => reject(r.error ?? new Error("could not read the file"));
    r.onload = () => {
      const url = String(r.result ?? "");
      const i = url.indexOf(",");
      resolve(i >= 0 ? url.slice(i + 1) : url);
    };
    r.readAsDataURL(blob);
  });
}
