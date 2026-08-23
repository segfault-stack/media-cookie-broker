import {instagramProvider} from "./instagram.js";
import {tiktokProvider} from "./tiktok.js";
import {xProvider} from "./x.js";
import {youtubeProvider} from "./youtube.js";

const providers = [youtubeProvider, tiktokProvider, instagramProvider, xProvider];
const byID = new Map(providers.map(provider => [provider.id, provider]));

export function getProvider(id) {
  return byID.get(id);
}

export function listProviders() {
  return [...providers];
}

export function providerForCookieDomain(domain) {
  const bare = domain.replace(/^\./, "").toLowerCase();
  return providers.find(provider => provider.cookieDomains.some(root => bare === root || bare.endsWith(`.${root}`)));
}
