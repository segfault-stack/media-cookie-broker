import {ALL_FORMATS, FilePathSource, Input} from 'mediabunny';

for (const filePath of process.argv.slice(2)) {
  const input = new Input({formats: ALL_FORMATS, source: new FilePathSource(filePath)});
  const [duration, videoTrack] = await Promise.all([
    input.computeDuration(),
    input.getPrimaryVideoTrack(),
  ]);
  if (!videoTrack) throw new Error(`${filePath}: no video track found`);
  console.log(JSON.stringify({
    file: filePath,
    durationSeconds: duration,
    width: videoTrack.displayWidth,
    height: videoTrack.displayHeight,
  }));
  input.dispose();
}
