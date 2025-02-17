import React, {Component} from 'react';
import './app.css';
import Nav from "./components/nav/nav";
import StreamingBackend from "./streamingBackend";
import {createStore} from 'redux';
import {rootReducer} from './redux/redux';
import {BrowserRouter as Router, Redirect, Route, Switch, withRouter} from "react-router-dom";
import GimletClient from "./client/client";
import Repositories from "./views/repositories/repositories";
import APIBackend from "./apiBackend";
import Profile from "./views/profile/profile";
import Services from "./views/services/services";
import Repo from "./views/repo/repo";
import DeployStatus from "./components/deployStatus/deployStatus";


export default class App extends Component {
  constructor(props) {
    super(props);

    const store = createStore(rootReducer);
    const gimletClient = new GimletClient(
      (response) => {
        if (response.status === 401) {
          window.location.replace("/login");
        } else {
          console.log(`${response.status}: ${response.statusText} on ${response.path}`);
        }
      }
    );

    this.state = {
      store: store,
      gimletClient: gimletClient,
    }
  }

  render() {
    const {store, gimletClient} = this.state;

    const NavBar = withRouter(props => <Nav {...props} store={store}/>);
    const APIBackendWithLocation = withRouter(
      props => <APIBackend {...props} store={store} gimletClient={gimletClient}/>
    );
    const StreamingBackendWithLocation = withRouter(props => <StreamingBackend {...props} store={store}/>);
    const RepoWithRouting = withRouter(props => <Repo {...props} store={store} gimletClient={gimletClient}/>);
    const ServicesWithRouting = withRouter(props => <Services {...props} store={store}/>);
    const RepositoriesWithRouting = withRouter(props => <Repositories {...props} store={store} gimletClient={gimletClient}/>);

    return (
      <Router>
        <StreamingBackendWithLocation/>
        <APIBackendWithLocation/>

        <Route exact path="/">
          <Redirect to="/repositories"/>
        </Route>

        <div className="min-h-screen bg-gray-100">
          <NavBar/>
          <div className="py-10">
            <Switch>
              <Route path="/services">
                <ServicesWithRouting/>
              </Route>

              <Route path="/repositories">
                <RepositoriesWithRouting/>
              </Route>

              <Route path="/profile">
                <Profile store={store}/>
              </Route>

              <Route path="/login">
                <button
                  type="button"
                  onClick={() => {
                    window.location.replace("/auth");
                  }}
                  className="inline-flex items-center px-6 py-3 border border-transparent text-base font-medium rounded-md
                 text-indigo-700 bg-indigo-100 hover:bg-indigo-200 focus:outline-none focus:ring-2 focus:ring-offset-2
                  focus:ring-indigo-500">
                  Login
                </button>
              </Route>

              <Route path="/repo/:owner/:repo">
                <RepoWithRouting store={store}/>
              </Route>

            </Switch>
          </div>
        </div>
        <DeployStatus store={store}/>
      </Router>
    )
  }
}
