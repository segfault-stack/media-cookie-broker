import {Composition} from 'remotion';
import {MediaCookieBrokerDemo} from './MediaCookieBrokerDemo';

export const Root: React.FC = () => (
  <Composition
    id="MediaCookieBrokerDemo"
    component={MediaCookieBrokerDemo}
    durationInFrames={390}
    fps={30}
    width={1280}
    height={720}
  />
);
