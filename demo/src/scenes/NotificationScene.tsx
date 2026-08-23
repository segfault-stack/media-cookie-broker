import {Easing, interpolate, useCurrentFrame} from 'remotion';
import {DesktopStage} from '../components/DesktopStage';
import {SystemNotification} from '../components/SystemNotification';
import {clamp} from '../theme';

export const NotificationScene: React.FC = () => {
  const frame = useCurrentFrame();
  return <div style={{position: 'absolute', inset: 0, scale: interpolate(frame, [0, 35], [1, 1.012], {...clamp, easing: Easing.bezier(0.16, 1, 0.3, 1)}), translate: `${interpolate(frame, [0, 35], [0, -7], clamp)}px 0`}}>
    <DesktopStage consumer="failure" broker="attention17" extension="attention17"><SystemNotification start={3} /></DesktopStage>
  </div>;
};
