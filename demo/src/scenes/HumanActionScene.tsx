import {Easing, interpolate, useCurrentFrame} from 'remotion';
import {Cursor} from '../components/Cursor';
import {DesktopStage} from '../components/DesktopStage';
import {clamp, colors, sans} from '../theme';

export const HumanActionScene: React.FC = () => {
  const frame = useCurrentFrame();
  return <DesktopStage consumer="failure" broker="attention17" extension="attention17" buttonPressed>
    <Cursor start={2} click={18} />
    <div style={{position: 'absolute', left: 838, top: 630, width: 330, textAlign: 'center', opacity: interpolate(frame, [21, 28], [0, 1], clamp), translate: `0 ${interpolate(frame, [21, 28], [8, 0], {...clamp, easing: Easing.bezier(0.16, 1, 0.3, 1)})}px`, fontFamily: sans, fontSize: 16, fontWeight: 800, color: '#c6e2ff'}}><span style={{color: colors.blue}}>Human-approved</span> recovery</div>
  </DesktopStage>;
};
