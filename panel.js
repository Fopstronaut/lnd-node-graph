const [dataNodes, dataEdges] = context.panel.data.series;
const [nodeIds, nodeNames, lastUpdates, highlighted, colors] = dataNodes.fields;
const [chanIds, srcs, dests, chanCapacity] = dataEdges.fields;

const nodeChannelLimit = [2, 30];
const depth = 2;

function truncateNumber(number, precision = 4) {
  return parseFloat(number.toPrecision(precision));
}

function nodeTooltipCallback({ data } = params) {
  return `${data.alias ?? data.name}<br>${data.value} BTC<br>${
    data.channels
  } channels`;
}

function getNodeNeighbors(node, depth = 0, nodes = [], edges = []) {
  if (!nodes.includes(node)) {
    nodes.push(node);
  }

  const neighbors = parsedEdges
    .filter((edge) => {
      if (edge.source === node.name || edge.target === node.name) {
        if (!edges.includes(edge)) {
          edges.push(edge);
        }
        return true;
      }
      return false;
    })
    .map((edge) => {
      let peerNode = undefined;
      if (edge.source === node.name) {
        peerNode = edge.target;
      } else if (edge.target === node.name) {
        peerNode = edge.source;
      } else {
        return false;
      }
      return parsedNodes.find((v) => v.name === peerNode) ?? false;
    })
    .filter(Boolean);

  if (depth > 0) {
    depth--;
    for (const neighbor of neighbors) {
      if (
        neighbor.channels < nodeChannelLimit[0] ||
        neighbor.channels > nodeChannelLimit[1]
      ) {
        continue;
      }
      getNodeNeighbors(neighbor, depth, nodes, edges);
    }
    nodes.push(...neighbors);
  }
  return [[...new Set(nodes).values()], [...new Set(edges).values()]];
}

let nodeIndiceCache = {};
const parsedNodes = dataNodes.first.map((_, i) => {
  const isHighlighted = highlighted.values[i] ? true : false;
  let node = {
    name: nodeIds.values[i],
    symbolSize: 12,
    value: 0,
    alias: nodeNames.values[i] ?? "",
    channels: 0,
    fixed: isHighlighted,
    itemStyle: {
      borderColor: colors.values[i],
      color: "#fff",
      borderWidth: 3,
    },
    label: {
      formatter: nodeNames.values[i] ?? "",
    },
    tooltip: {
      formatter: nodeTooltipCallback,
    },
  };
  if (isHighlighted) {
    node.x = context.panel.chart.getWidth() / 2;
    node.y = context.panel.chart.getHeight() / 2;
  }
  nodeIndiceCache[node.name] = i;
  return node;
});
const parsedEdges = dataEdges.first
  .map((_, i) => {
    const source = srcs.values[i];
    const target = dests.values[i];
    const capacity = chanCapacity.values[i];

    const srcIdx = nodeIndiceCache[source];
    const destIdx = nodeIndiceCache[target];
    if (!srcIdx || !destIdx) {
      return false;
    }

    const btcValue = truncateNumber(capacity / 1e8);
    parsedNodes[srcIdx].channels++;
    parsedNodes[srcIdx].value = truncateNumber(
      parsedNodes[srcIdx].value + btcValue
    );
    parsedNodes[srcIdx].symbolSize =
      Math.log(parsedNodes[srcIdx].channels) * 10;

    parsedNodes[destIdx].channels++;
    parsedNodes[destIdx].value = truncateNumber(
      parsedNodes[destIdx].value + btcValue
    );
    parsedNodes[destIdx].symbolSize =
      Math.log(parsedNodes[destIdx].channels) * 10;

    const lineWidth = 1 + Math.log(Math.log(capacity));
    return {
      source: source,
      target: target,
      value: capacity,
      tooltip: {
        formatter: `${chanIds.values[i]}<br>${Intl.NumberFormat().format(
          capacity
        )} sats`,
      },
      lineStyle: {
        width: lineWidth,
        color: parsedNodes[srcIdx].itemStyle.borderColor,
      },
    };
  })
  .filter(Boolean);

const ourNode = parsedNodes.find((v) => v.fixed);
const [nodes, edges] = getNodeNeighbors(ourNode, depth);

return {
  title: {
    backgroundColor: "#444c",
    text: `${nodes.length - 1} nodes, ${
      edges.length
    } channels within ${depth} hops`,
    textStyle: {
      fontSize: 12,
      fontWeight: "normal",
    },
    subtext: `${parsedNodes.length} nodes, ${parsedEdges.length} channels total`,
  },
  tooltip: {
    textStyle: {
      color: "#fff",
    },
    backgroundColor: "#44444444",
    borderWidth: 0,
    trigger: "item",
  },
  series: [
    {
      type: "graph",
      layout: "force",
      zoom: 0.3,
      nodeScaleRatio: 1,
      selectedMode: "multiple",
      autoCurveness: true,
      animation: false,
      roam: true,
      draggable: false,
      data: nodes,
      links: edges,
      edgeSymbol: ["circle", "arrow"],
      edgeSymbolSize: 6,
      emphasis: {
        focus: "adjacency",
        lineStyle: {
          width: 6,
        },
      },
      force: {
        edgeLength: 30,
        repulsion: 10000,
        gravity: 0.1,
        friction: 0.3,
      },
      labelLayout: {
        hideOverlap: true,
      },
      label: {
        show: true,
        position: "bottom",
      },
      lineStyle: {
        curveness: 0.3,
      },
      select: {
        lineStyle: {
          color: "#fff",
          width: 6,
        },
        label: {
          show: true,
        },
      },
    },
  ],
};
