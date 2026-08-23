import {Easing, interpolate, useCurrentFrame} from 'remotion';
import {DesktopStage} from '../components/DesktopStage';
import {clamp, colors, sans} from '../theme';

export const PayoffScene: React.FC = () => {
  const frame = useCurrentFrame();
  return <DesktopStage consumer="healthy18" broker="healthy18" extension="healthy18">
    <div style={{position: 'absolute', left: 0, right: 0, bottom: 45, textAlign: 'center', opacity: interpolate(frame, [5, 15], [0, 1], clamp), translate: `0 ${interpolate(frame, [5, 15], [10, 0], {...clamp, easing: Easing.bezier(0.16, 1, 0.3, 1)})}px`, fontFamily: sans}}>
      <div style={{fontSize: 25, lineHeight: 1.25, fontWeight: 800, letterSpacing: -0.35, color: colors.text}}>Browser auth when a human is needed. <span style={{color: colors.green}}>Plain cookies.txt everywhere else.</span></div>
      <div style={{marginTop: 10, fontSize: 14, fontWeight: 750, letterSpacing: 0.4, color: colors.muted}}>🍪 MEDIA COOKIE BROKER</div>
    </div>
  </DesktopStage>;
};
