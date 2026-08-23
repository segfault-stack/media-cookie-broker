import {Easing, interpolate, useCurrentFrame} from 'remotion';
import {clamp} from '../theme';
import {DesktopStage} from '../components/DesktopStage';

export const HealthyScene: React.FC = () => {
  const frame = useCurrentFrame();
  return <div style={{position: 'absolute', inset: 0, opacity: interpolate(frame, [0, 10], [0, 1], clamp), scale: interpolate(frame, [0, 14], [0.985, 1], {...clamp, easing: Easing.bezier(0.16, 1, 0.3, 1)})}}><DesktopStage consumer="healthy17" broker="healthy17" extension="healthy17" /></div>;
};
