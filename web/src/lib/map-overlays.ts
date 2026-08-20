import L from 'leaflet';

const MIENVIRO_SERVICE = 'https://gisagoegle.state.mi.us/arcgis/services/EGLE/MiEnviro/MapServer/WMSServer';

export const DEFAULT_OVERLAYS: MapOverlay[] = [
  { label: 'Part 201 Sites', url: MIENVIRO_SERVICE, layerIds: [8] },
  { label: 'CWS Intake Locations', url: MIENVIRO_SERVICE, layerIds: [52] },
  { label: 'Cold/Transitional Streams', url: MIENVIRO_SERVICE, layerIds: [1] },
  { label: 'Part 115 Landfills', url: MIENVIRO_SERVICE, layerIds: [39] },
];

export type MapOverlay = {
  label: string;
  url: string;
  layerIds: number[];
  opacity?: number;
};

export function configuredOverlays(): MapOverlay[] | null {
  const raw = import.meta.env.VITE_MAP_OVERLAYS;
  if (!raw) return null;
  const overlays: MapOverlay[] = [];
  for (const entry of raw.split(';')) {
    const [label, url, ids, opacity] = entry.split('|').map((part) => part.trim());
    if (!label || !url || !ids) continue;
    const layerIds = ids.split(',').map(Number).filter(Number.isInteger);
    if (layerIds.length === 0) continue;
    const parsedOpacity = opacity ? Number(opacity) : undefined;
    overlays.push({ label, url, layerIds, opacity: parsedOpacity && parsedOpacity > 0 ? parsedOpacity : undefined });
  }
  return overlays.length > 0 ? overlays : null;
}

class WmsTileLayer extends L.TileLayer {
  getTileUrl(coords: L.Coords): string {
    const url = super.getTileUrl(coords);
    const bounds = (this._map as L.Map | undefined)?.getBounds();
    if (!bounds) return url;
    return `${url}&BBOX=${bounds.toBBoxString()}`;
  }
}

export function wmsOverlayLayer(overlay: MapOverlay): L.TileLayer {
  const params = new URLSearchParams({
    SERVICE: 'WMS',
    VERSION: '1.1.1',
    REQUEST: 'GetMap',
    LAYERS: overlay.layerIds.join(','),
    STYLES: '',
    FORMAT: 'image/png',
    TRANSPARENT: 'TRUE',
    SRS: 'EPSG:3857',
    WIDTH: '256',
    HEIGHT: '256',
  });
  return new WmsTileLayer(`${overlay.url}?${params.toString()}`, {
    opacity: overlay.opacity ?? 0.85,
    maxZoom: 19,
  });
}