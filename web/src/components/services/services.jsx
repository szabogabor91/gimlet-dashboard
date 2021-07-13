import React, {Component} from 'react';
import './services.css';

export default class Services extends Component {
  constructor(props) {
    super(props);

    // default state
    this.state = {
      envs: {}
    }

    // handling API and streaming state changes
    this.props.store.subscribe(() => {
      let reduxState = this.props.store.getState();

      this.setState({envs: reduxState.envs});
    });
  }

  render() {
    const {envs} = this.state;

    if (!envs.loaded) {
      return (
        <p>loading</p>
      )
    }

    return (
      <ul className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
        {envs['staging'].stacks.map((service) => {
          console.log(service);
          return (
            <li key={service.name} className="col-span-1 bg-white rounded-lg shadow divide-y divide-gray-200">
              <Service {...service}/>
            </li>
          )
        })}
      </ul>
    );

  }

}

function Service(service) {
  return (
    <div className="w-full flex items-center justify-between p-6 space-x-6">
      <div className="flex-1 truncate">
        <div className="flex items-center space-x-3">
          <h3 className="text-gray-900 text-sm font-medium truncate">{service.repo}</h3>
        </div>
        {service.deployments.map((deployment) => (
            <div>
              <p className="text-xs">{deployment.namespace}/{deployment.name}
                <span
                  className="flex-shrink-0 inline-block px-2 py-0.5 mx-1 text-green-800 text-xs font-medium bg-green-100 rounded-full">
                  {deployment.env}
                </span>
              </p>
              <p className="mb-1">
                <p className="break-words">{deployment.message}</p>
                <p className="text-xs italic">
                  <a
                    href="https://github.com" target="_blank"
                    rel="noopener noreferrer">
                    {deployment.sha.slice(0, 6)}
                  </a>
                </p>
              </p>
              {deployment.pods.map((pod) => (
                <Pod pod={pod}/>
              ))}
            </div>
          )
        )}
      </div>
    </div>
  )
}

function Pod(props) {
  const {pod} = props;

  let color;
  let pulsar;
  switch (pod.status) {
    case 'Running':
      color = 'text-blue-200';
      pulsar = '';
      break;
    case 'PodInitializing':
    case 'ContainerCreating':
    case 'Pending':
      color = 'text-blue-100';
      pulsar = 'pulsar-green';
      break;
    case 'Terminating':
      color = 'text-blue-800';
      pulsar = 'pulsar-gray';
      break;
    default:
      color = 'text-red-600';
      pulsar = '';
      break;
  }

  return (
    <span className="inline-block w-4 mr-1 mt-2">
      <svg viewBox="0 0 1 1"
           className={`fill-current ${color}, ${pulsar}`}>
        <g>
          <title>{pod.name} - {pod.status}</title>
          <rect width="1" height="1"/>
        </g>
      </svg>
    </span>
  );
}
