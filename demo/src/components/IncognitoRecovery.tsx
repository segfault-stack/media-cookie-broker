import {Easing, interpolate, useCurrentFrame} from 'remotion';
import {clamp, colors, sans} from '../theme';
import {BrowserWindow} from './BrowserWindow';
import {StatusPill} from './StatusPill';

const CheckRow: React.FC<{children: React.ReactNode; showAt: number}> = ({children, showAt}) => {
  const frame = useCurrentFrame();
  return <div style={{display: 'flex', alignItems: 'center', gap: 12, opacity: interpolate(frame, [showAt, showAt + 8], [0, 1], clamp), translate: `0 ${interpolate(frame, [showAt, showAt + 8], [10, 0], {...clamp, easing: Easing.bezier(0.16, 1, 0.3, 1)})}px`, color: '#dff5e5', fontSize: 18, fontWeight: 700}}><span style={{width: 27, height: 27, borderRadius: 99, display: 'grid', placeItems: 'center', background: colors.greenBg, border: '1px solid #36784a', color: colors.green}}>✓</span>{children}</div>;
};

export const IncognitoRecovery: React.FC = () => {
  const frame = useCurrentFrame();
  return <div style={{position: 'absolute', left: 225, top: 76, width: 830, height: 555, opacity: interpolate(frame, [0, 10, 67, 75], [0, 1, 1, 0], clamp), scale: interpolate(frame, [0, 12, 67, 75], [0.96, 1, 1, 0.97], {...clamp, easing: Easing.bezier(0.16, 1, 0.3, 1)})}}>
    <BrowserWindow incognito title="Incognito recovery · browser interaction">
      <div style={{height: 'calc(100% - 50px)', display: 'grid', gridTemplateColumns: '1fr 1fr', background: 'linear-gradient(145deg,#16141d,#10151c)', fontFamily: sans}}>
        <div style={{padding: '53px 45px', borderRight: '1px solid #393445'}}>
          <StatusPill tone="browser">Isolated browser context</StatusPill>
          <div style={{fontSize: 36, lineHeight: 1.12, fontWeight: 850, color: colors.text, marginTop: 23, letterSpacing: -1.1}}>Incognito<br />recovery</div>
          <div style={{fontSize: 21, color: '#d6cffa', marginTop: 17, fontWeight: 750}}>YouTube / default</div>
          <div style={{marginTop: 29, padding: '14px 15px', borderRadius: 11, background: '#24202d', border: '1px solid #4b4459', color: '#d9d4e4', fontSize: 14, lineHeight: 1.55}}>A person completes the provider flow in this browser window.</div>
        </div>
        <div style={{padding: '53px 42px'}}>
          <div style={{fontSize: 14, letterSpacing: 1.5, fontWeight: 800, color: colors.muted}}>HUMAN-DRIVEN STEP</div>
          <div style={{fontSize: 25, fontWeight: 800, color: colors.text, marginTop: 12}}>Interactive login</div>
          <div style={{fontSize: 17, color: '#d5bffd', marginTop: 7}}>CAPTCHA / 2FA if required</div>
          <div style={{marginTop: 38, display: 'grid', gap: 18}}>
            <CheckRow showAt={29}>browser session ready</CheckRow>
            <CheckRow showAt={48}>YouTube-scoped cookies captured</CheckRow>
          </div>
          <div style={{height: 4, borderRadius: 4, overflow: 'hidden', marginTop: 40, background: '#2d2935'}}><div style={{height: '100%', width: `${interpolate(frame, [8, 58], [4, 100], clamp)}%`, borderRadius: 4, background: 'linear-gradient(90deg,#6faeff,#9a8cff)'}} /></div>
        </div>
      </div>
    </BrowserWindow>
  </div>;
};
