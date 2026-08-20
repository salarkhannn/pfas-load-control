import { describe, expect, it } from 'vitest';
import type L from 'leaflet';

import { wmsOverlayLayer } from '@/lib/map-overlays';

function tileUrl(layer: L.TileLayer, x = 0, y = 0, z = 0): string {
  (layer as L.TileLayer & { _map: L.Map; _globalTileRange: { max: { y: number } } })._map = {
    getZoom: () => 5,
    options: {
      crs: {
        infinite: false,
        getProjectedBounds: () => ({ max: { x: 20037508.34, y: 20037508.34 } }),
      },
    },
    getBounds: () => ({ toBBoxString: () => '-83.9,41.8,-83.8,41.9' }),
  } as unknown as L.Map;
  (layer as L.TileLayer & { _globalTileRange: { max: { y: number } } })._globalTileRange = { max: { y: 10 } };
  return layer.getTileUrl({ x, y, z } as unknown as L.Coords) as string;
}

describe('wmsOverlayLayer', () => {
  it('builds a WMS GetMap tile URL with the requested ArcGIS layers', () => {
    const layer = wmsOverlayLayer({
      label: 'Part 201 Sites',
      url: 'https://gisagoegle.state.mi.us/arcgis/services/EGLE/MiEnviro/MapServer/WMSServer',
      layerIds: [8],
    });
    const url = tileUrl(layer, 10, 20, 5);
    expect(url).toContain('SERVICE=WMS');
    expect(url).toContain('VERSION=1.1.1');
    expect(url).toContain('REQUEST=GetMap');
    expect(url).toContain('LAYERS=8');
    expect(url).toContain('TRANSPARENT=TRUE');
    expect(url).toContain('SRS=EPSG%3A3857');
    expect(url).toContain('&BBOX=-83.9,41.8,-83.8,41.9');
  });

  it('joins multiple layer ids and honors opacity', () => {
    const layer = wmsOverlayLayer({ label: 'Streams', url: 'https://example.test/wms', layerIds: [1, 2], opacity: 0.5 });
    expect(tileUrl(layer)).toContain('LAYERS=1%2C2');
    expect(layer.options.opacity).toBe(0.5);
  });
});