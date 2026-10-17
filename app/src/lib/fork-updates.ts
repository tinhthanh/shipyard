// This fork's releases carry berth and berthd only, with no signed app
// and no latest.json beside them, so the updater's check finds nothing to
// read. That isn't a failure to report: the app here is built from source.

export function noAppRelease(error: string): boolean {
  return /valid release JSON|latest\.json/i.test(error) || /\b404\b/.test(error);
}

export const NO_APP_RELEASE_LINE = "Fork build: update the app from source";
