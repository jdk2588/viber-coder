"""Models."""

import dataclasses
from typing import Dict, List


@dataclasses.dataclass(frozen=True)
class DiscoveredBulb:
    """Representation of discovered bulb."""

    ip_address: str
    mac_address: str


def _bulb_map_factory() -> Dict[str, DiscoveredBulb]:
    """Create the mapping used to store bulbs by their MAC address."""

    return {}


@dataclasses.dataclass
class BulbRegistry:
    """Representation of the bulb registry."""

    bulbs_by_mac: Dict[str, DiscoveredBulb] = dataclasses.field(
        default_factory=_bulb_map_factory
    )

    def register(self, bulb: DiscoveredBulb) -> None:
        """Register a new bulb."""
        self.bulbs_by_mac[bulb.mac_address] = bulb

    def bulbs(self) -> List[DiscoveredBulb]:
        """Get all present bulbs."""
        return list(self.bulbs_by_mac.values())
