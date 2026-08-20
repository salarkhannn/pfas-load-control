import { useEffect, useRef } from 'react';
import L from 'leaflet';
import type { GeoJsonObject } from 'geojson';

import 'leaflet/dist/leaflet.css';

import { configuredOverlays, DEFAULT_OVERLAYS, wmsOverlayLayer, type MapOverlay } from '@/lib/map-overlays';

const DEFAULT_TILE_URL = 'https://tile.openstreetmap.org/{z}/{x}/{y}.png';
const DEFAULT_ATTRIBUTION = '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors';

export function ParcelMap({ geometry, latitude, longitude, label, samples = [], overlays }: {
  geometry: unknown;
  latitude?: string;
  longitude?: string;
  label: string;
  samples?: Array<{ index: number; label: string; latitude: number; longitude: number }>;
  overlays?: MapOverlay[];
}) {
  const container = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!container.current) return;
    const parcel = geoJSON(geometry);
    if (!parcel) return;

    const map = L.map(container.current, {
      attributionControl: true,
      scrollWheelZoom: false,
      zoomControl: true,
    });
    const styles = getComputedStyle(document.documentElement);
    const accent = styles.getPropertyValue('--accent-solid').trim();
    const detail = styles.getPropertyValue('--brand-mark-detail').trim();
    const surface = styles.getPropertyValue('--surface-default').trim();
    const foreground = styles.getPropertyValue('--fg-default').trim();
    L.tileLayer(import.meta.env.VITE_MAP_TILE_URL || DEFAULT_TILE_URL, {
      attribution: import.meta.env.VITE_MAP_ATTRIBUTION || DEFAULT_ATTRIBUTION,
      maxZoom: 19,
    }).addTo(map);

    const activeOverlays = overlays ?? configuredOverlays() ?? DEFAULT_OVERLAYS;
    const overlayControls: Record<string, L.Layer> = {};
    activeOverlays.forEach((overlay) => {
      const layer = wmsOverlayLayer(overlay);
      overlayControls[overlay.label] = layer;
      layer.addTo(map);
    });
    L.control.layers(undefined, overlayControls, { position: 'topright', collapsed: true }).addTo(map);

    const parcelLayer = L.geoJSON(parcel, {
      style: {
        color: accent,
        fillColor: detail,
        fillOpacity: 0.24,
        opacity: 1,
        weight: 3,
      },
    }).addTo(map);
    map.fitBounds(parcelLayer.getBounds(), { padding: [28, 28], maxZoom: 17 });

    const lat = Number(latitude);
    const lng = Number(longitude);
    if (Number.isFinite(lat) && Number.isFinite(lng)) {
      L.circleMarker([lat, lng], {
        color: surface,
        fillColor: foreground,
        fillOpacity: 1,
        radius: 5,
        weight: 2,
      }).bindTooltip(label).addTo(map);
    }

    samples.forEach((sample) => {
      if (!Number.isFinite(sample.latitude) || !Number.isFinite(sample.longitude)) return;
      const markerContent = document.createElement('span');
      markerContent.textContent = String(sample.index + 1);
      L.marker([sample.latitude, sample.longitude], {
        icon: L.divIcon({
          className: 'field-sample-marker',
          html: markerContent,
          iconAnchor: [11, 11],
          iconSize: [22, 22],
        }),
      }).bindTooltip(sample.label).addTo(map);
    });

    const observer = new ResizeObserver(() => map.invalidateSize({ pan: false }));
    observer.observe(container.current);
    return () => {
      observer.disconnect();
      map.remove();
    };
  }, [geometry, label, latitude, longitude, samples, overlays]);

  if (!geoJSON(geometry)) return null;
  const sampleCopy = samples.length ? ` with ${samples.length} evidence points` : '';
  return <div className="parcel-map" ref={container} role="region" aria-label={`Parcel boundary map for ${label}${sampleCopy}`} />;
}

function geoJSON(value: unknown): GeoJsonObject | null {
  if (!value || typeof value !== 'object') return null;
  const candidate = value as { type?: unknown; coordinates?: unknown };
  if ((candidate.type !== 'Polygon' && candidate.type !== 'MultiPolygon') || !Array.isArray(candidate.coordinates)) return null;
  return candidate as GeoJsonObject;
}